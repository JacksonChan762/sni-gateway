package services

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"sni-gateway/internal/db"
	"sni-gateway/internal/models"

	"github.com/robfig/cron/v3"
)

type SubscriptionManager struct {
	cron    *cron.Cron
	entries map[uint]cron.EntryID
}

func NewSubscriptionManager() *SubscriptionManager {
	return &SubscriptionManager{
		cron:    cron.New(),
		entries: make(map[uint]cron.EntryID),
	}
}

func (sm *SubscriptionManager) Start() {
	sm.cron.Start()
	sm.loadExistingSubscriptions()
}

func (sm *SubscriptionManager) Stop() {
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
	if entry, ok := sm.entries[sub.ID]; ok {
		sm.cron.Remove(entry)
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
	if entry, ok := sm.entries[subID]; ok {
		sm.cron.Remove(entry)
		delete(sm.entries, subID)
	}
}

func (sm *SubscriptionManager) SyncSubscription(subID uint) error {
	var sub models.Subscription
	if err := db.DB.First(&sub, subID).Error; err != nil {
		return err
	}

	log.Printf("Syncing subscription %d: %s", subID, sub.URL)

	// Fetch the .list file
	resp, err := http.Get(sub.URL)
	if err != nil {
		sm.updateSyncStatus(&sub, "failed", err.Error(), 0)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		msg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		sm.updateSyncStatus(&sub, "failed", msg, 0)
		return fmt.Errorf(msg)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		sm.updateSyncStatus(&sub, "failed", err.Error(), 0)
		return err
	}

	// Parse rules
	parsed := ParseListFile(string(body))

	// Delete old subscription rules
	if err := db.DB.Where("source = 'subscription' AND sub_id = ?", sub.ID).Delete(&models.Rule{}).Error; err != nil {
		sm.updateSyncStatus(&sub, "failed", err.Error(), 0)
		return err
	}

	// Insert new rules
	for _, pr := range parsed {
		rule := models.Rule{
			Pattern: pr.Pattern,
			Type:    pr.Type,
			ExitID:  sub.ExitID,
			Source:  "subscription",
			SubID:   &sub.ID,
			Enabled: true,
			Priority: 50, // subscription rules have lower priority than manual
		}
		db.DB.Create(&rule)
	}

	sm.updateSyncStatus(&sub, "success", "", len(parsed))
	log.Printf("Subscription %d synced: %d rules", subID, len(parsed))
	return nil
}

func (sm *SubscriptionManager) updateSyncStatus(sub *models.Subscription, status, errMsg string, count int) {
	now := time.Now()
	sub.LastSyncAt = &now
	sub.LastSyncStatus = status
	sub.LastError = errMsg
	db.DB.Save(sub)
}
