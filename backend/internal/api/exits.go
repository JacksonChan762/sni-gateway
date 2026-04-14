package api

import (
	"encoding/json"
	"sni-gateway/internal/db"
	"sni-gateway/internal/models"
	"sni-gateway/internal/services"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

type ExitHandler struct {
	manager *services.ExitManager
}

func NewExitHandler(mgr *services.ExitManager) *ExitHandler {
	return &ExitHandler{manager: mgr}
}

func (h *ExitHandler) List(c *fiber.Ctx) error {
	var exits []models.Exit
	if err := db.DB.Find(&exits).Error; err != nil {
		return err
	}
	// Scrub sensitive fields
	for i := range exits {
		exits[i].ConfigJSON = ""
	}
	return c.JSON(exits)
}

type CreateExitRequest struct {
	Name               string `json:"name"`
	Runtime            string `json:"runtime"`
	Protocol           string `json:"protocol"`
	Provider           string `json:"provider"`
	Country            string `json:"country"`
	City               string `json:"city"`
	WireguardPrivKey   string `json:"wireguard_private_key"`
	WireguardAddresses string `json:"wireguard_addresses"`
	OpenVPNUser        string `json:"openvpn_user"`
	OpenVPNPassword    string `json:"openvpn_password"`
	Enabled            *bool  `json:"enabled"`
}

func (h *ExitHandler) Create(c *fiber.Ctx) error {
	var req CreateExitRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name is required"})
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	runtime := req.Runtime
	if runtime == "" {
		runtime = "gluetun"
	}

	// Store credentials in config_json
	creds := map[string]string{}
	if req.WireguardPrivKey != "" {
		creds["wireguard_private_key"] = req.WireguardPrivKey
	}
	if req.WireguardAddresses != "" {
		creds["wireguard_addresses"] = req.WireguardAddresses
	}
	if req.OpenVPNUser != "" {
		creds["openvpn_user"] = req.OpenVPNUser
	}
	if req.OpenVPNPassword != "" {
		creds["openvpn_password"] = req.OpenVPNPassword
	}
	credsJSON, _ := json.Marshal(creds)

	exit := models.Exit{
		Name:       req.Name,
		Runtime:    runtime,
		Protocol:   req.Protocol,
		Provider:   req.Provider,
		Country:    req.Country,
		City:       req.City,
		Enabled:    enabled,
		Status:     "stopped",
		ConfigJSON: string(credsJSON),
	}
	if err := db.DB.Create(&exit).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	// Fire off container creation
	go func() {
		if err := h.manager.CreateAndStart(&exit); err != nil {
			// Status already updated by manager
		}
	}()

	exit.ConfigJSON = ""
	return c.Status(201).JSON(exit)
}

func (h *ExitHandler) Update(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))
	var exit models.Exit
	if err := db.DB.First(&exit, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}

	var req CreateExitRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	if req.Protocol != "" {
		exit.Protocol = req.Protocol
	}
	if req.Provider != "" {
		exit.Provider = req.Provider
	}
	if req.Country != "" {
		exit.Country = req.Country
	}
	if req.City != "" {
		exit.City = req.City
	}
	if req.Enabled != nil {
		exit.Enabled = *req.Enabled
	}

	if err := db.DB.Save(&exit).Error; err != nil {
		return err
	}
	exit.ConfigJSON = ""
	return c.JSON(exit)
}

func (h *ExitHandler) Delete(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))
	var exit models.Exit
	if err := db.DB.First(&exit, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}

	// Check if rules depend on this exit
	var count int64
	db.DB.Model(&models.Rule{}).Where("exit_id = ? AND enabled = true", id).Count(&count)
	if count > 0 {
		return c.Status(400).JSON(fiber.Map{
			"error": "cannot delete exit with active rules",
			"rule_count": count,
		})
	}

	_ = h.manager.Delete(&exit)
	db.DB.Delete(&exit)
	return c.JSON(fiber.Map{"message": "deleted"})
}

func (h *ExitHandler) Start(c *fiber.Ctx) error {
	exit := h.getExit(c)
	if exit == nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	if err := h.manager.Start(exit); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "started"})
}

func (h *ExitHandler) Stop(c *fiber.Ctx) error {
	exit := h.getExit(c)
	if exit == nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	if err := h.manager.Stop(exit); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "stopped"})
}

func (h *ExitHandler) Restart(c *fiber.Ctx) error {
	exit := h.getExit(c)
	if exit == nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	if err := h.manager.Restart(exit); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "restarted"})
}

func (h *ExitHandler) Test(c *fiber.Ctx) error {
	exit := h.getExit(c)
	if exit == nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	status, ip, err := h.manager.GetStatus(exit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"status":    status,
		"public_ip": ip,
	})
}

func (h *ExitHandler) Logs(c *fiber.Ctx) error {
	exit := h.getExit(c)
	if exit == nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	tail := 100
	if t := c.QueryInt("tail", 0); t > 0 {
		tail = t
	}
	logs, err := h.manager.GetLogs(exit, tail)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"logs": logs})
}

func (h *ExitHandler) Status(c *fiber.Ctx) error {
	exit := h.getExit(c)
	if exit == nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	status, ip, err := h.manager.GetStatus(exit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	// Update DB
	exit.Status = status
	if ip != "" {
		exit.PublicIP = ip
	}
	db.DB.Save(exit)
	return c.JSON(fiber.Map{
		"status":    status,
		"public_ip": ip,
	})
}

func (h *ExitHandler) getExit(c *fiber.Ctx) *models.Exit {
	id, _ := strconv.Atoi(c.Params("id"))
	var exit models.Exit
	if err := db.DB.First(&exit, id).Error; err != nil {
		return nil
	}
	return &exit
}
