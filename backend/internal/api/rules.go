package api

import (
	"sni-gateway/internal/db"
	"sni-gateway/internal/models"
	"strconv"
	"strings"

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
	if source := c.Query("source", "manual"); source != "" && source != "all" {
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

type CreateRulesBulkRequest struct {
	Rules []CreateRuleRequest `json:"rules"`
}

func (h *RuleHandler) Create(c *fiber.Ctx) error {
	var req CreateRuleRequest
	if _, err := decodeJSONBody(c.Body(), &req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	rule, err := buildManualRule(req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if err := ensureExitExists(db.DB, rule.ExitID); err != nil {
		return exitReferenceResponse(c, err)
	}
	if err := db.DB.Create(&rule).Error; err != nil {
		return err
	}
	db.DB.Preload("Exit").First(&rule, rule.ID)
	return c.Status(201).JSON(rule)
}

func (h *RuleHandler) CreateBulk(c *fiber.Ctx) error {
	var req CreateRulesBulkRequest
	if _, err := decodeJSONBody(c.Body(), &req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if len(req.Rules) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "rules are required"})
	}
	if len(req.Rules) > 5000 {
		return c.Status(400).JSON(fiber.Map{"error": "cannot create more than 5000 rules at once"})
	}

	seen := make(map[string]bool, len(req.Rules))
	rules := make([]models.Rule, 0, len(req.Rules))
	for i, item := range req.Rules {
		rule, err := buildManualRule(item)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "rule " + strconv.Itoa(i+1) + ": " + err.Error()})
		}
		if err := ensureExitExists(db.DB, rule.ExitID); err != nil {
			return exitReferenceResponse(c, err)
		}

		key := rule.Type + "\x00" + rule.Pattern
		if seen[key] {
			continue
		}
		seen[key] = true
		rules = append(rules, rule)
	}
	if len(rules) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "no unique rules to create"})
	}

	if err := db.DB.Create(&rules).Error; err != nil {
		return err
	}
	ids := make([]uint, 0, len(rules))
	for _, rule := range rules {
		ids = append(ids, rule.ID)
	}
	if err := db.DB.Preload("Exit").Where("id IN ?", ids).Order("id ASC").Find(&rules).Error; err != nil {
		return err
	}

	return c.Status(201).JSON(fiber.Map{
		"created_count": len(rules),
		"rules":         rules,
	})
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

	if req.Pattern != "" || req.Type != "" {
		rawPattern := rule.Pattern
		if req.Pattern != "" {
			rawPattern = req.Pattern
			rule.Pattern = normalizeRulePattern(req.Pattern)
		}

		requestedType := rule.Type
		if req.Type != "" {
			requestedType = req.Type
		}
		if req.Type == "auto" && req.Pattern == "" {
			requestedType = rule.Type
		}

		ruleType, err := resolveManualRuleType(rawPattern, requestedType)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		rule.Type = ruleType
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

func buildManualRule(req CreateRuleRequest) (models.Rule, error) {
	pattern := normalizeRulePattern(req.Pattern)
	if pattern == "" {
		return models.Rule{}, fiber.NewError(fiber.StatusBadRequest, "pattern is required")
	}

	ruleType := strings.TrimSpace(req.Type)
	ruleType, err := resolveManualRuleType(req.Pattern, ruleType)
	if err != nil {
		return models.Rule{}, err
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	priority := req.Priority
	if priority == 0 {
		priority = 100
	}

	return models.Rule{
		Pattern:  pattern,
		Type:     ruleType,
		ExitID:   req.ExitID,
		Priority: priority,
		Source:   "manual",
		Enabled:  enabled,
	}, nil
}

func normalizeRulePattern(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	pattern = strings.TrimPrefix(pattern, "+.")
	return strings.TrimSpace(pattern)
}

func resolveManualRuleType(pattern string, ruleType string) (string, error) {
	ruleType = strings.TrimSpace(ruleType)
	if strings.HasPrefix(strings.TrimSpace(pattern), "+.") {
		return "suffix", nil
	}
	if ruleType == "" || ruleType == "auto" {
		return "exact", nil
	}
	if ruleType != "exact" && ruleType != "suffix" {
		return "", fiber.NewError(fiber.StatusBadRequest, "type must be exact, suffix, or auto")
	}
	return ruleType, nil
}

func exitReferenceResponse(c *fiber.Ctx, err error) error {
	if err == gorm.ErrRecordNotFound {
		return c.Status(400).JSON(fiber.Map{"error": "exit_id does not reference an existing exit"})
	}
	return err
}
