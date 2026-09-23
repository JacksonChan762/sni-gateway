package api

import (
	"sni-gateway/internal/catalog"

	"github.com/gofiber/fiber/v2"
)

func listVPNProviders(c *fiber.Ctx) error {
	providers, commit, err := catalog.ListProviders()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"providers": providers, "source_commit": commit})
}

func searchVPNServers(c *fiber.Ctx) error {
	page, err := catalog.Search(c.Query("provider"), c.Query("protocol"), c.Query("country"), c.Query("query"), c.QueryInt("offset", 0), c.QueryInt("limit", 50))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(page)
}
