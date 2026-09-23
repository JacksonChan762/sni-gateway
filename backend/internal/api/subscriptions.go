package api

import (
	"sni-gateway/internal/db"
	"sni-gateway/internal/models"
	"sni-gateway/internal/services"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type SubscriptionHandler struct {
	manager   *services.SubscriptionManager
	generator *services.ConfigGenerator
}

func NewSubscriptionHandler(mgr *services.SubscriptionManager, gen *services.ConfigGenerator) *SubscriptionHandler {
	return &SubscriptionHandler{manager: mgr, generator: gen}
}

func (h *SubscriptionHandler) List(c *fiber.Ctx) error {
	var subs []models.Subscription
	if err := db.DB.Preload("Exit", publicExit).Find(&subs).Error; err != nil {
		return err
	}
	// Count all subscriptions in one query instead of one query per row.
	var counts []struct {
		SubID uint
		Count int64
	}
	if err := db.DB.Model(&models.Rule{}).Select("sub_id, COUNT(*) AS count").
		Where("source = ? AND sub_id IS NOT NULL", "subscription").Group("sub_id").Scan(&counts).Error; err != nil {
		return err
	}
	byID := make(map[uint]int, len(counts))
	for _, row := range counts {
		byID[row.SubID] = int(row.Count)
	}
	for i := range subs {
		subs[i].RuleCount = byID[subs[i].ID]
	}

	return c.JSON(subs)
}

type CreateSubRequest struct {
	Name            string `json:"name"`
	URL             string `json:"url"`
	ExitID          *uint  `json:"exit_id"`
	Enabled         *bool  `json:"enabled"`
	IntervalMinutes int    `json:"interval_minutes"`
}

func (h *SubscriptionHandler) Create(c *fiber.Ctx) error {
	var req CreateSubRequest
	if _, err := decodeJSONBody(c.Body(), &req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if req.Name == "" || req.URL == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name and url are required"})
	}
	if err := ensureExitExists(db.DB, req.ExitID); err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(400).JSON(fiber.Map{"error": "exit_id does not reference an existing exit"})
		}
		return err
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.IntervalMinutes < 0 {
		return c.Status(400).JSON(fiber.Map{"error": "interval_minutes must be positive"})
	}
	interval := req.IntervalMinutes
	if interval == 0 {
		interval = 360
	}

	sub := models.Subscription{
		Name:            req.Name,
		URL:             req.URL,
		ExitID:          req.ExitID,
		Enabled:         enabled,
		IntervalMinutes: interval,
	}
	if err := db.DB.Create(&sub).Error; err != nil {
		return err
	}
	if sub.Enabled {
		h.manager.Schedule(sub)
		if err := h.manager.SyncSubscription(sub.ID); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
	}
	if err := db.DB.Preload("Exit", publicExit).First(&sub, sub.ID).Error; err != nil {
		return err
	}
	return c.Status(201).JSON(sub)
}

func (h *SubscriptionHandler) Update(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))
	var sub models.Subscription
	if err := db.DB.First(&sub, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}

	var req CreateSubRequest
	fields, err := decodeJSONBody(c.Body(), &req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	if req.IntervalMinutes < 0 {
		return c.Status(400).JSON(fiber.Map{"error": "interval_minutes must be positive"})
	}
	if req.Name != "" {
		sub.Name = req.Name
	}
	if req.URL != "" {
		sub.URL = req.URL
	}
	if _, ok := fields["exit_id"]; ok {
		present, exitID, err := parseOptionalUintField(fields, "exit_id")
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if present {
			if err := ensureExitExists(db.DB, exitID); err != nil {
				if err == gorm.ErrRecordNotFound {
					return c.Status(400).JSON(fiber.Map{"error": "exit_id does not reference an existing exit"})
				}
				return err
			}
			sub.ExitID = exitID
		}
	}
	if req.Enabled != nil {
		sub.Enabled = *req.Enabled
	}
	if req.IntervalMinutes > 0 {
		sub.IntervalMinutes = req.IntervalMinutes
	}

	if err := db.DB.Save(&sub).Error; err != nil {
		return err
	}

	if sub.Enabled {
		h.manager.Schedule(sub)
		if err := h.manager.SyncSubscription(sub.ID); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
	} else {
		h.manager.Unschedule(sub.ID)
		if err := db.DB.Where("source = 'subscription' AND sub_id = ?", sub.ID).Delete(&models.Rule{}).Error; err != nil {
			return err
		}
		if h.generator != nil {
			if _, err := h.generator.Apply(); err != nil {
				return c.Status(500).JSON(fiber.Map{"error": err.Error()})
			}
		}
	}

	if err := db.DB.Preload("Exit", publicExit).First(&sub, sub.ID).Error; err != nil {
		return err
	}
	return c.JSON(sub)
}

func (h *SubscriptionHandler) Delete(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))
	var sub models.Subscription
	if err := db.DB.First(&sub, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}

	h.manager.Unschedule(sub.ID)

	// Delete associated rules
	if err := db.DB.Where("source = 'subscription' AND sub_id = ?", sub.ID).Delete(&models.Rule{}).Error; err != nil {
		return err
	}

	if err := db.DB.Delete(&sub).Error; err != nil {
		return err
	}
	if h.generator != nil {
		if _, err := h.generator.Apply(); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
	}
	return c.JSON(fiber.Map{"message": "deleted"})
}

func (h *SubscriptionHandler) Sync(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))
	if err := h.manager.SyncSubscription(uint(id)); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "sync completed"})
}
