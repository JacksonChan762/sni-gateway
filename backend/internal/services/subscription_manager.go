package services

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"sni-gateway/internal/db"
	"sni-gateway/internal/models"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SubscriptionManager struct {
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	syncMu    sync.Mutex
	client    *http.Client
	cron      *cron.Cron
	entries   map[uint]cron.EntryID
	configGen *ConfigGenerator
}

func NewSubscriptionManager(configGen *ConfigGenerator) *SubscriptionManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &SubscriptionManager{
		ctx: ctx, cancel: cancel,
		client:    &http.Client{Timeout: 30 * time.Second},
		cron:      cron.New(),
		entries:   make(map[uint]cron.EntryID),
		configGen: configGen,
	}
}

func (sm *SubscriptionManager) Start() {
	sm.cron.Start()
	sm.loadExistingSubscriptions()
}

func (sm *SubscriptionManager) Stop() {
	sm.cancel()
	sm.cron.Stop()
}

func (sm *SubscriptionManager) loadExistingSubscriptions() {
	var subs []models.Subscription
	if err := db.DB.Where("enabled = true").Find(&subs).Error; err != nil {
		log.Printf("Failed to load subscriptions: %v", err)
		return
	}
	for _, sub := range subs {
		sm.Schedule(sub)
	}
}

func (sm *SubscriptionManager) Schedule(sub models.Subscription) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if entry, ok := sm.entries[sub.ID]; ok {
		sm.cron.Remove(entry)
	}

	delete(sm.entries, sub.ID)
	if !sub.Enabled || sub.IntervalMinutes <= 0 {
		return
	}
	spec := fmt.Sprintf("@every %dm", sub.IntervalMinutes)
	id, err := sm.cron.AddFunc(spec, func() {
		sm.SyncSubscription(sub.ID)
	})
	if err != nil {
		log.Printf("Failed to schedule subscription %d: %v", sub.ID, err)
		return
	}
	sm.entries[sub.ID] = id
}

func (sm *SubscriptionManager) Unschedule(subID uint) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if entry, ok := sm.entries[subID]; ok {
		sm.cron.Remove(entry)
		delete(sm.entries, subID)
	}
}

func (sm *SubscriptionManager) SyncSubscription(subID uint) error {
	// Serialize syncs so their database replacements and publishes cannot overlap.
	sm.syncMu.Lock()
	defer sm.syncMu.Unlock()
	if err := sm.ctx.Err(); err != nil {
		return err
	}
	var sub models.Subscription
	if err := db.DB.First(&sub, subID).Error; err != nil {
		return err
	}

	log.Printf("Syncing subscription %d: %s", subID, sub.URL)

	if !sub.Enabled {
		return fmt.Errorf("subscription %d is disabled", subID)
	}
	body, err := fetchSubscriptionContext(sm.ctx, sm.client, sub.URL)
	if err != nil {
		sm.updateSyncStatus(&sub, "failed", err.Error())
		return err
	}
	parsed := ParseListFile(string(body))
	changed := false
	// Keep IDs for unchanged rules so equal-priority overlaps remain stable.
	err = db.DB.Transaction(func(tx *gorm.DB) error {
		// Discard downloads made stale by edits, disabling, or deletion.
		var current models.Subscription
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, sub.ID).Error; err != nil {
			return err
		}
		if !current.Enabled || !current.UpdatedAt.Equal(sub.UpdatedAt) {
			return fmt.Errorf("subscription changed during download; retry sync")
		}
		var existing []models.Rule
		if err := tx.Select("id", "pattern", "type", "exit_id", "priority", "enabled").
			Where("source = 'subscription' AND sub_id = ?", sub.ID).Order("id ASC").Find(&existing).Error; err != nil {
			return err
		}
		plan := planSubscriptionSync(existing, parsed, sub.ID, sub.ExitID)
		changed = len(plan.insert) > 0 || len(plan.remove) > 0 || len(plan.update) > 0
		for start := 0; start < len(plan.remove); start += 500 {
			end := min(start+500, len(plan.remove))
			if err := tx.Where("id IN ?", plan.remove[start:end]).Delete(&models.Rule{}).Error; err != nil {
				return err
			}
		}
		for start := 0; start < len(plan.update); start += 500 {
			end := min(start+500, len(plan.update))
			if err := tx.Model(&models.Rule{}).Where("id IN ?", plan.update[start:end]).Updates(map[string]interface{}{
				"exit_id": sub.ExitID, "priority": 50, "enabled": true,
			}).Error; err != nil {
				return err
			}
		}
		if len(plan.insert) > 0 {
			return tx.CreateInBatches(&plan.insert, 500).Error
		}
		return nil
	})
	if err != nil {
		sm.updateSyncStatus(&sub, "failed", err.Error())
		return err
	}

	// Retry a previous publish failure even when the downloaded list is unchanged.
	needsPublish := changed || strings.HasPrefix(sub.LastError, "publish failed:")
	if needsPublish && sm.configGen != nil {
		if _, err := sm.configGen.Apply(); err != nil {
			sm.updateSyncStatus(&sub, "failed", fmt.Sprintf("publish failed: %v", err))
			return err
		}
	}

	sm.updateSyncStatus(&sub, "success", "")
	log.Printf("Subscription %d synced: %d rules", subID, len(parsed))
	return nil
}

type subscriptionSyncPlan struct {
	insert []models.Rule
	update []uint
	remove []uint
}

func planSubscriptionSync(existing []models.Rule, parsed []ParsedRule, subID uint, exitID *uint) subscriptionSyncPlan {
	plan := subscriptionSyncPlan{}
	byKey := make(map[ParsedRule]models.Rule, len(existing))
	for _, rule := range existing {
		key := ParsedRule{Pattern: rule.Pattern, Type: rule.Type}
		if _, present := byKey[key]; present {
			plan.remove = append(plan.remove, rule.ID)
		} else {
			byKey[key] = rule
		}
	}
	for _, key := range parsed {
		if rule, present := byKey[key]; present {
			delete(byKey, key)
			if !sameExitID(rule.ExitID, exitID) || rule.Priority != 50 || !rule.Enabled {
				plan.update = append(plan.update, rule.ID)
			}
		} else {
			plan.insert = append(plan.insert, models.Rule{
				Pattern: key.Pattern, Type: key.Type, ExitID: exitID,
				Source: "subscription", SubID: &subID, Enabled: true, Priority: 50,
			})
		}
	}
	for _, rule := range byKey {
		plan.remove = append(plan.remove, rule.ID)
	}
	return plan
}

func sameExitID(a, b *uint) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func (sm *SubscriptionManager) updateSyncStatus(sub *models.Subscription, status, errMsg string) {
	now := time.Now()
	sub.LastSyncAt = &now
	sub.LastSyncStatus = status
	sub.LastError = errMsg
	// Update only sync metadata, preserving concurrent edits and deleted rows.
	if err := db.DB.Model(&models.Subscription{}).Where("id = ?", sub.ID).Updates(map[string]interface{}{
		"last_sync_at": now, "last_sync_status": status, "last_error": errMsg,
	}).Error; err != nil {
		log.Printf("Failed to update subscription %d sync status: %v", sub.ID, err)
	}
}

const maxSubscriptionBytes = 10 * 1024 * 1024

func fetchSubscription(client *http.Client, url string) ([]byte, error) {
	return fetchSubscriptionContext(context.Background(), client, url)
}

func fetchSubscriptionContext(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSubscriptionBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxSubscriptionBytes {
		return nil, fmt.Errorf("subscription exceeds 10 MiB limit")
	}
	return body, nil
}
