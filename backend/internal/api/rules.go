package api

import (
	"sni-gateway/internal/db"
	"sni-gateway/internal/models"
	"sni-gateway/internal/services"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type RuleHandler struct{ publisher services.Publisher }

func NewRuleHandler(publisher services.Publisher) *RuleHandler {
	return &RuleHandler{publisher: publisher}
}

func (h *RuleHandler) List(c *fiber.Ctx) error {
	rules := []models.Rule{}
	q := db.DB.Preload("Exit", publicExit)
	if source := c.Query("source", "manual"); source != "" && source != "all" {
		q = q.Where("source = ?", source)
	}
	if exitID := c.Query("exit_id"); exitID != "" {
		q = q.Where("exit_id = ?", exitID)
	}
	if folderFilter := c.Query("folder_id"); folderFilter != "" {
		q = q.Where("source = ?", "manual")
		if folderFilter == "unfiled" {
			q = q.Where("folder_id IS NULL")
		} else {
			folderID, ok := parseRuleFolderID(folderFilter)
			if !ok {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "folder_id must be a positive integer or unfiled"})
			}
			q = q.Where("folder_id = ?", folderID)
		}
	}
	// Keep the original array response for existing API clients. The UI opts
	// into a bounded result with page, limit, and q.
	if c.Query("page") != "" || c.Query("limit") != "" || c.Query("q") != "" {
		page, pageErr := strconv.Atoi(c.Query("page", "1"))
		limit, limitErr := strconv.Atoi(c.Query("limit", "50"))
		if pageErr != nil || limitErr != nil || page < 1 || page > 1000000 || limit < 1 || limit > 200 {
			return c.Status(400).JSON(fiber.Map{"error": "page must be 1–1000000 and limit must be 1–200"})
		}
		search := strings.TrimSpace(c.Query("q"))
		if len(search) > 253 {
			return c.Status(400).JSON(fiber.Map{"error": "search is too long"})
		}
		if search != "" {
			term := "%" + escapeLike(search) + "%"
			exitIDs := db.DB.Model(&models.Exit{}).Select("id").Where("name ILIKE ?", term)
			q = q.Where("pattern ILIKE ? OR exit_id IN (?)", term, exitIDs)
		}
		var total int64
		if err := q.Model(&models.Rule{}).Count(&total).Error; err != nil {
			return err
		}
		if err := q.Order("priority DESC, id ASC").Offset((page - 1) * limit).Limit(limit).Find(&rules).Error; err != nil {
			return err
		}
		return c.JSON(fiber.Map{"rules": rules, "total": total, "page": page, "limit": limit})
	}
	q = q.Order("priority DESC, id ASC")
	if err := q.Find(&rules).Error; err != nil {
		return err
	}
	return c.JSON(rules)
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	return strings.ReplaceAll(value, `_`, `\_`)
}

type CreateRuleRequest struct {
	Pattern  string `json:"pattern"`
	Type     string `json:"type"`
	ExitID   *uint  `json:"exit_id"`
	Priority int    `json:"priority"`
	Enabled  *bool  `json:"enabled"`
	FolderID *uint  `json:"folder_id"`
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
	if err := ensureRuleFoldersExist([]models.Rule{rule}); err != nil {
		return ruleFolderReferenceResponse(c, err)
	}
	if err := db.DB.Create(&rule).Error; err != nil {
		return ruleFolderReferenceResponse(c, err)
	}
	h.publisher.InvalidatePending()
	db.DB.Preload("Exit", publicExit).First(&rule, rule.ID)
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
	if err := ensureRuleExitsExist(db.DB, rules); err != nil {
		return exitReferenceResponse(c, err)
	}
	if err := ensureRuleFoldersExist(rules); err != nil {
		return ruleFolderReferenceResponse(c, err)
	}

	if err := db.DB.Create(&rules).Error; err != nil {
		return ruleFolderReferenceResponse(c, err)
	}
	h.publisher.InvalidatePending()
	ids := make([]uint, 0, len(rules))
	for _, rule := range rules {
		ids = append(ids, rule.ID)
	}
	if err := db.DB.Preload("Exit", publicExit).Where("id IN ?", ids).Order("id ASC").Find(&rules).Error; err != nil {
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
	previous := rule

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
		if !services.ValidDomain(rule.Pattern) {
			return c.Status(400).JSON(fiber.Map{"error": "pattern must be a valid ASCII or punycode domain"})
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
	present, folderID, err := parseOptionalUintField(fields, "folder_id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if present && !sameRuleReference(previous.FolderID, folderID) {
		if folderID != nil && rule.Source != "manual" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "only manual rules can be added to a folder"})
		}
		if !validOptionalRuleFolderID(folderID) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "folder_id must be a positive folder ID or null"})
		}
		rule.FolderID = folderID
		if err := ensureRuleFoldersExist([]models.Rule{rule}); err != nil {
			return ruleFolderReferenceResponse(c, err)
		}
	}

	updates := changedRoutingFields(previous, rule)
	routingChanged := len(updates) > 0
	if !sameRuleReference(previous.FolderID, rule.FolderID) {
		updates["folder_id"] = rule.FolderID
	}
	if len(updates) > 0 {
		query := db.DB.Model(&models.Rule{ID: rule.ID})
		var result *gorm.DB
		if routingChanged {
			result = query.Updates(updates)
		} else {
			// Keep the routing snapshot's update timestamp unchanged for an organizational move.
			result = query.UpdateColumns(updates)
		}
		if result.Error != nil {
			return ruleFolderReferenceResponse(c, result.Error)
		}
		if result.RowsAffected == 0 {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
		}
		if routingChanged {
			h.publisher.InvalidatePending()
		}
	}
	var updated models.Rule
	if err := db.DB.Preload("Exit", publicExit).First(&updated, rule.ID).Error; err != nil {
		return err
	}
	return c.JSON(updated)
}

func changedRoutingFields(before, after models.Rule) map[string]any {
	changes := make(map[string]any)
	if before.Pattern != after.Pattern {
		changes["pattern"] = after.Pattern
	}
	if before.Type != after.Type {
		changes["type"] = after.Type
	}
	if !sameRuleReference(before.ExitID, after.ExitID) {
		changes["exit_id"] = after.ExitID
	}
	if before.Priority != after.Priority {
		changes["priority"] = after.Priority
	}
	if before.Enabled != after.Enabled {
		changes["enabled"] = after.Enabled
	}
	return changes
}

func sameRuleReference(a, b *uint) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
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
	h.publisher.InvalidatePending()
	return c.JSON(fiber.Map{"message": "deleted"})
}

func buildManualRule(req CreateRuleRequest) (models.Rule, error) {
	if !validOptionalRuleFolderID(req.FolderID) {
		return models.Rule{}, fiber.NewError(fiber.StatusBadRequest, "folder_id must be a positive folder ID or null")
	}
	pattern := normalizeRulePattern(req.Pattern)
	if !services.ValidDomain(pattern) {
		return models.Rule{}, fiber.NewError(fiber.StatusBadRequest, "pattern must be a valid ASCII or punycode domain")
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
		FolderID: req.FolderID,
	}, nil
}

func normalizeRulePattern(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	pattern = strings.TrimPrefix(pattern, "+.")
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(pattern), "."))
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
