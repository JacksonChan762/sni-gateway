package api

import (
	"github.com/gofiber/fiber/v2"
	"net"
	"sni-gateway/internal/db"
	"sni-gateway/internal/models"
	"sni-gateway/internal/services"
	"strings"
)

// Test previews saved rules. It does not claim that pending changes are published.
func (h *RuleHandler) Test(c *fiber.Ctx) error {
	var req struct {
		Host string `json:"host"`
	}
	if _, err := decodeJSONBody(c.Body(), &req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(req.Host), "."))
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = strings.TrimSuffix(name, ".")
	}
	if !services.ValidDomain(host) {
		return c.Status(400).JSON(fiber.Map{"error": "enter a domain name, optionally followed by a port"})
	}
	var rules []models.Rule
	var exits []models.Exit
	if err := db.DB.Where("enabled = true").Find(&rules).Error; err != nil {
		return err
	}
	if err := db.DB.Where("enabled = true").Find(&exits).Error; err != nil {
		return err
	}
	table, err := services.NewRoutingTable(rules, exits)
	if err != nil {
		return err
	}
	rule := table.Match(host)
	if rule == nil {
		return c.JSON(fiber.Map{"matched": false, "host": host})
	}
	var selected models.Exit
	for _, exit := range exits {
		if exit.ID == *rule.ExitID {
			selected = exit
			break
		}
	}
	return c.JSON(fiber.Map{"matched": true, "host": host,
		"rule": fiber.Map{"id": rule.ID, "pattern": rule.Pattern, "type": rule.Type, "priority": rule.Priority},
		"exit": fiber.Map{"id": selected.ID, "name": selected.Name, "status": selected.Status},
	})
}
