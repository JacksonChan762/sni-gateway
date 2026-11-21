package services

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
	"sni-gateway/internal/db"
	"sni-gateway/internal/models"
	"sni-gateway/internal/provider"
	"sni-gateway/internal/singbox"
)

type Publisher interface {
	Preview() (string, error)
	Apply() (string, error)
	Rollback() error
	HasPendingChanges() (bool, error)
	InvalidatePending()
	WriteHistory(version, status, message, config string)
}

type routeSnapshot struct {
	Rules []models.Rule `json:"rules"`
	Exits []models.Exit `json:"exits"`
}

// RoutePublisher keeps the active table separate from edits in the database.
type RoutePublisher struct {
	mu           sync.Mutex // serializes publication, rollback, and pending checks
	path         string
	active       atomic.Pointer[RoutingTable]
	pending      bool
	pendingUntil time.Time
}

func NewRoutePublisher(path string) (*RoutePublisher, error) {
	p := &RoutePublisher{path: path}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		data = []byte(`{"rules":[],"exits":[]}`)
	} else if err != nil {
		return nil, err
	}
	table, err := tableFromSnapshot(data)
	if err != nil {
		return nil, fmt.Errorf("load published routes: %w", err)
	}
	p.active.Store(table)
	return p, nil
}

func tableFromSnapshot(data []byte) (*RoutingTable, error) {
	var snapshot routeSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, err
	}
	return NewRoutingTable(snapshot.Rules, snapshot.Exits)
}

func (p *RoutePublisher) snapshot() ([]byte, error) {
	var snapshot routeSnapshot
	var configuredExits []models.Exit
	err := db.DB.Transaction(func(tx *gorm.DB) error {
		// Folder assignments belong to the Rules page and carry no routing settings.
		if err := tx.Omit("FolderID").Where("enabled = true").Order("priority DESC, id ASC").Find(&snapshot.Rules).Error; err != nil {
			return err
		}
		return tx.Where("enabled = true").Order("id ASC").Find(&configuredExits).Error
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	exitByID := make(map[uint]models.Exit, len(configuredExits))
	for _, exit := range configuredExits {
		exitByID[exit.ID] = exit
		snapshot.Exits = append(snapshot.Exits, models.Exit{ID: exit.ID, Name: exit.Name, Runtime: exit.Runtime, Enabled: exit.Enabled})
	}
	checked := make(map[uint]bool)
	for _, rule := range snapshot.Rules {
		if rule.ExitID == nil {
			continue
		}
		if checked[*rule.ExitID] {
			continue
		}
		exit, exists := exitByID[*rule.ExitID]
		if exists {
			if exit.Runtime != "sing-box" && exit.Runtime != "gluetun" {
				return nil, fmt.Errorf("rule %d uses legacy exit %q; edit it with a complete WireGuard configuration", rule.ID, exit.Name)
			}
			var err error
			if exit.Runtime == "gluetun" {
				err = provider.Validate(exit)
			} else {
				_, err = singbox.BuildExitConfig(exit)
			}
			if err != nil {
				return nil, fmt.Errorf("rule %d uses invalid exit %q: %w", rule.ID, exit.Name, err)
			}
			checked[*rule.ExitID] = true
		}
	}
	if _, err := NewRoutingTable(snapshot.Rules, snapshot.Exits); err != nil {
		return nil, err
	}
	return json.MarshalIndent(snapshot, "", "  ")
}

func (p *RoutePublisher) Preview() (string, error) {
	data, err := p.snapshot()
	return string(data), err
}

func (p *RoutePublisher) Apply() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	data, err := p.snapshot()
	if err != nil {
		return "", err
	}
	table, err := tableFromSnapshot(data)
	if err != nil {
		return "", err
	}
	old, readErr := os.ReadFile(p.path)
	if readErr != nil && !os.IsNotExist(readErr) {
		return "", readErr
	}
	if err := os.MkdirAll(filepath.Dir(p.path), 0700); err != nil {
		return "", err
	}
	if readErr == nil {
		if err := writePrivateFile(p.path+".bak", old); err != nil {
			return "", err
		}
	}
	if err := writePrivateFile(p.path, data); err != nil {
		return "", err
	}
	p.active.Store(table)
	p.pending = false
	p.pendingUntil = time.Now().Add(30 * time.Second)
	return string(data), nil
}

func (p *RoutePublisher) Rollback() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	data, err := os.ReadFile(p.path + ".bak")
	if err != nil {
		return err
	}
	table, err := tableFromSnapshot(data)
	if err != nil {
		return err
	}
	current, err := os.ReadFile(p.path)
	if err != nil {
		return err
	}
	if err := writePrivateFile(p.path, data); err != nil {
		return err
	}
	if err := writePrivateFile(p.path+".bak", current); err != nil {
		return errors.Join(err, writePrivateFile(p.path, current))
	}
	p.active.Store(table)
	p.pendingUntil = time.Time{}
	return nil
}

func (p *RoutePublisher) HasPendingChanges() (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if time.Now().Before(p.pendingUntil) {
		return p.pending, nil
	}
	data, err := p.snapshot()
	if err != nil {
		return false, err
	}
	current, err := os.ReadFile(p.path)
	if os.IsNotExist(err) {
		p.pending = true
		p.pendingUntil = time.Now().Add(30 * time.Second)
		return p.pending, nil
	}
	if err != nil {
		return false, err
	}
	p.pending = string(current) != string(data)
	p.pendingUntil = time.Now().Add(30 * time.Second)
	return p.pending, nil
}

func (p *RoutePublisher) ExitName(host string) string {
	return p.active.Load().ExitName(host)
}

func (p *RoutePublisher) InvalidatePending() {
	p.mu.Lock()
	p.pendingUntil = time.Time{}
	p.mu.Unlock()
}

func (p *RoutePublisher) WriteHistory(version, status, message, config string) {
	db.DB.Create(&models.PublishHistory{Version: version, Status: status, Message: message, Config: config})
}

func writePrivateFile(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".routes-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
