package api

import (
	"sni-gateway/internal/db"
	"sni-gateway/internal/models"
	"sni-gateway/internal/services"
	"strconv"

	"github.com/gofiber/fiber/v2"
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
	if err := db.DB.Preload("Exit").Find(&subs).Error; err != nil {
		return err
	}
	// Attach rule counts
	for i := range subs {
		var count int64
		db.DB.Model(&models.Rule{}).
			Where("source = 'subscription' AND sub_id = ?", subs[i].ID).
			Count(&count)
		subs[i].RuleCount = int(count)
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
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if req.Name == "" || req.URL == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name and url are required"})
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
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
	db.DB.Preload("Exit").First(&sub, sub.ID)

	if sub.Enabled {
		if err := h.manager.SyncSubscription(sub.ID); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		h.manager.Schedule(sub)
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
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	if req.Name != "" {
		sub.Name = req.Name
	}
	if req.URL != "" {
		sub.URL = req.URL
	}
	if req.ExitID != nil {
		sub.ExitID = req.ExitID
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
		if err := h.manager.SyncSubscription(sub.ID); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		h.manager.Schedule(sub)
	} else {
		h.manager.Unschedule(sub.ID)
		if err := db.DB.Where("source = 'subscription' AND sub_id = ?", sub.ID).Delete(&models.Rule{}).Error; err != nil {
			return err
		}
		if h.generator != nil {
			if err := h.generator.WriteAndReload(""); err != nil {
				return c.Status(500).JSON(fiber.Map{"error": err.Error()})
			}
		}
	}

	db.DB.Preload("Exit").First(&sub, sub.ID)
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
		if err := h.generator.WriteAndReload(""); err != nil {
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
