package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"sni-gateway/internal/db"
	"sni-gateway/internal/models"
)

type containerStopper interface {
	StopContainer(context.Context, string) error
}

type containerRemover interface {
	RemoveContainer(context.Context, string, bool) error
}

// Always attempt both components, even if the first operation fails.
func operateExitContainers(name string, operation func(string) error) error {
	var failures []error
	for _, prefix := range []string{"relay-", "gluetun-"} {
		if err := operation(prefix + name); err != nil {
			failures = append(failures, fmt.Errorf("%s%s: %w", prefix, name, err))
		}
	}
	return errors.Join(failures...)
}

func stopExitContainers(ctx context.Context, client containerStopper, name string) error {
	return operateExitContainers(name, func(container string) error { return client.StopContainer(ctx, container) })
}

func removeExitContainers(ctx context.Context, client containerRemover, name string) error {
	return operateExitContainers(name, func(container string) error { return client.RemoveContainer(ctx, container, true) })
}

func (em *ExitManager) stopWithContext(ctx context.Context, exit *models.Exit) error {
	if err := em.ensureDockerAvailable(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 70*time.Second)
	defer cancel()
	em.logEvent(exit.ID, "stop", "started", "")
	if err := stopExitContainers(ctx, em.docker, exit.Name); err != nil {
		em.updateStatus(exit, "unhealthy")
		em.logEvent(exit.ID, "stop", "failed", err.Error())
		return err
	}
	if err := db.DB.Model(&models.Exit{}).Where("id = ?", exit.ID).Updates(map[string]interface{}{"status": "stopped", "public_ip": ""}).Error; err != nil {
		return err
	}
	exit.Status, exit.PublicIP = "stopped", ""
	em.logEvent(exit.ID, "stop", "success", "VPN and relay stopped")
	return nil
}

type ExitStopResult struct {
	ID    uint   `json:"id"`
	Name  string `json:"name"`
	Error string `json:"error,omitempty"`
}

func (em *ExitManager) StopAll(ctx context.Context) ([]ExitStopResult, error) {
	em.operationMu.Lock()
	defer em.operationMu.Unlock()
	if err := em.ensureDockerAvailable(); err != nil {
		return nil, err
	}
	var exits []models.Exit
	if err := db.DB.Order("id ASC").Find(&exits).Error; err != nil {
		return nil, err
	}
	results := make([]ExitStopResult, len(exits))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range jobs {
				results[i] = ExitStopResult{ID: exits[i].ID, Name: exits[i].Name}
				if err := em.stopWithContext(ctx, &exits[i]); err != nil {
					results[i].Error = err.Error()
				}
			}
		}()
	}
	for i := range exits {
		jobs <- i
	}
	close(jobs)
	workers.Wait()
	return results, nil
}
