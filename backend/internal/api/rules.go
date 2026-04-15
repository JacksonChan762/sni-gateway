package api

import (
	"sni-gateway/internal/db"
	"sni-gateway/internal/models"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type RuleHandler struct{}

func NewRuleHandler() *RuleHandler {
	return &RuleHandler{}
}

func (h *RuleHandler) List(c *fiber.Ctx) error {
	var rules []models.Rule
	q := db.DB.Preload("Exit")
	if source := c.Query("source"); source != "" {
		q = q.Where("source = ?", source)
	}
	if exitID := c.Query("exit_id"); exitID != "" {
		q = q.Where("exit_id = ?", exitID)
	}
	q = q.Order("priority DESC, id ASC")
	if err := q.Find(&rules).Error; err != nil {
		return err
	}
	return c.JSON(rules)
}

type CreateRuleRequest struct {
	Pattern  string `json:"pattern"`
	Type     string `json:"type"`
	ExitID   *uint  `json:"exit_id"`
	Priority int    `json:"priority"`
	Enabled  *bool  `json:"enabled"`
}

func (h *RuleHandler) Create(c *fiber.Ctx) error {
	var req CreateRuleRequest
	if _, err := decodeJSONBody(c.Body(), &req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if req.Pattern == "" {
		return c.Status(400).JSON(fiber.Map{"error": "pattern is required"})
	}
	if req.Type != "exact" && req.Type != "suffix" {
		return c.Status(400).JSON(fiber.Map{"error": "type must be exact or suffix"})
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
	priority := req.Priority
	if priority == 0 {
		priority = 100
	}

	rule := models.Rule{
		Pattern:  req.Pattern,
		Type:     req.Type,
		ExitID:   req.ExitID,
		Priority: priority,
		Source:   "manual",
		Enabled:  enabled,
	}
	if err := db.DB.Create(&rule).Error; err != nil {
		return err
	}
	db.DB.Preload("Exit").First(&rule, rule.ID)
	return c.Status(201).JSON(rule)
}

func (h *RuleHandler) Update(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))
	var rule models.Rule
	if err := db.DB.First(&rule, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}

	var req CreateRuleRequest
	fields, err := decodeJSONBody(c.Body(), &req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	if req.Pattern != "" {
		rule.Pattern = req.Pattern
	}
	if req.Type != "" {
		rule.Type = req.Type
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
			rule.ExitID = exitID
		}
	}
	if req.Priority != 0 {
		rule.Priority = req.Priority
	}
	if req.Enabled != nil {
		rule.Enabled = *req.Enabled
	}

	if err := db.DB.Save(&rule).Error; err != nil {
		return err
	}
	db.DB.Preload("Exit").First(&rule, rule.ID)
	return c.JSON(rule)
}

func (h *RuleHandler) Delete(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))
	var rule models.Rule
	if err := db.DB.First(&rule, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	if err := db.DB.Delete(&rule).Error; err != nil {
		return err
	}
	return c.JSON(fiber.Map{"message": "deleted"})
}
