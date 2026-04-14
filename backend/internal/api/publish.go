package api

import (
	"fmt"
	"sni-gateway/internal/db"
	"sni-gateway/internal/models"
	"sni-gateway/internal/services"
	"time"

	"github.com/gofiber/fiber/v2"
)

type PublishHandler struct {
	generator *services.ConfigGenerator
}

func NewPublishHandler(gen *services.ConfigGenerator) *PublishHandler {
	return &PublishHandler{generator: gen}
}

func (h *PublishHandler) Preview(c *fiber.Ctx) error {
	config, err := h.generator.Preview()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"config": config})
}

func (h *PublishHandler) Validate(c *fiber.Ctx) error {
	_, err := h.generator.Preview()
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"valid": false, "error": err.Error()})
	}
	return c.JSON(fiber.Map{"valid": true})
}

func (h *PublishHandler) Apply(c *fiber.Ctx) error {
	config, err := h.generator.Generate()
	if err != nil {
		h.recordHistory("failed", err.Error(), config)
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.generator.WriteAndReload(config); err != nil {
		h.recordHistory("failed", err.Error(), config)
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	h.recordHistory("success", "config applied successfully", config)
	return c.JSON(fiber.Map{"message": "config applied and nginx reloaded"})
}

func (h *PublishHandler) Rollback(c *fiber.Ctx) error {
	if err := h.generator.Rollback(); err != nil {
		h.recordHistory("failed", "rollback failed: "+err.Error(), "")
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	h.recordHistory("rollback", "rolled back to previous config", "")
	return c.JSON(fiber.Map{"message": "rolled back to previous config"})
}

func (h *PublishHandler) History(c *fiber.Ctx) error {
	var history []models.PublishHistory
	if err := db.DB.Order("created_at DESC").Limit(50).Find(&history).Error; err != nil {
		return err
	}
	// Scrub config content for list view
	for i := range history {
		history[i].Config = ""
	}
	return c.JSON(history)
}

func (h *PublishHandler) recordHistory(status, message, config string) {
	version := fmt.Sprintf("v%d", time.Now().Unix())
	h.generator.WriteHistory(version, status, message, config)
}

func getDashboardStats(c *fiber.Ctx) error {
	var ruleCount, subCount, exitCount, runningExitCount int64

	db.DB.Model(&models.Rule{}).Where("enabled = true AND source = 'manual'").Count(&ruleCount)
	db.DB.Model(&models.Subscription{}).Where("enabled = true").Count(&subCount)
	db.DB.Model(&models.Exit{}).Where("enabled = true").Count(&exitCount)
	db.DB.Model(&models.Exit{}).Where("status = 'running'").Count(&runningExitCount)

	var lastPublish models.PublishHistory
	db.DB.Order("created_at DESC").First(&lastPublish)

	return c.JSON(fiber.Map{
		"rule_count":         ruleCount,
		"sub_count":          subCount,
		"exit_count":         exitCount,
		"running_exit_count": runningExitCount,
		"last_publish":       lastPublish,
	})
}
