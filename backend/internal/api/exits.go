package api

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"sni-gateway/internal/db"
	"sni-gateway/internal/models"
	"sni-gateway/internal/services"

	"github.com/gofiber/fiber/v2"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
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
	WireguardPrivKey      string `json:"wireguard_private_key"`
	WireguardPublicKey    string `json:"wireguard_public_key"`
	WireguardEndpointIP   string `json:"wireguard_endpoint_ip"`
	WireguardEndpointPort string `json:"wireguard_endpoint_port"`
	WireguardPresharedKey string `json:"wireguard_preshared_key"`
	WireguardAllowedIPs   string `json:"wireguard_allowed_ips"`
	WireguardAddresses    string `json:"wireguard_addresses"`
	OpenVPNUser           string `json:"openvpn_user"`
	OpenVPNPassword       string `json:"openvpn_password"`
	ConfigYAML            string `json:"config_yaml"`
	Enabled               *bool  `json:"enabled"`
}

func (h *ExitHandler) Create(c *fiber.Ctx) error {
	var req CreateExitRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Runtime = strings.TrimSpace(req.Runtime)
	req.Protocol = strings.TrimSpace(req.Protocol)
	req.Provider = strings.TrimSpace(req.Provider)
	req.Country = strings.TrimSpace(req.Country)
	req.City = strings.TrimSpace(req.City)
	req.ConfigYAML = strings.TrimSpace(req.ConfigYAML)

	if req.ConfigYAML != "" {
		if err := applyYAMLToExitRequest(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
	}

	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name is required"})
	}
	if req.Protocol == "" {
		req.Protocol = "wireguard"
	}
	if req.Provider == "" {
		req.Provider = "protonvpn"
	}
	if req.Protocol != "wireguard" && req.Protocol != "openvpn" {
		return c.Status(400).JSON(fiber.Map{"error": "protocol must be wireguard or openvpn"})
	}
	if err := validateExitRequest(req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	runtime := req.Runtime
	if runtime == "" {
		runtime = "gluetun"
	}

	configJSON, err := buildExitConfigJSON(req)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	exit := models.Exit{
		Name:       req.Name,
		Runtime:    runtime,
		Protocol:   req.Protocol,
		Provider:   req.Provider,
		Country:    req.Country,
		City:       req.City,
		Enabled:    enabled,
		Status:     "stopped",
		ConfigJSON: configJSON,
	}
	if err := db.DB.Create(&exit).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	if enabled {
		if err := h.manager.CreateAndStart(&exit); err != nil {
			db.DB.Delete(&exit)
			return c.Status(500).JSON(fiber.Map{
				"error": fmt.Sprintf("failed to create exit runtime: %v", err),
			})
		}
	}

	exit.ConfigJSON = ""
	return c.Status(201).JSON(exit)
}

func (h *ExitHandler) Update(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))
	var exit models.Exit
	if err := db.DB.First(&exit, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	before := exit

	var req CreateExitRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Runtime = strings.TrimSpace(req.Runtime)
	req.Protocol = strings.TrimSpace(req.Protocol)
	req.Provider = strings.TrimSpace(req.Provider)
	req.Country = strings.TrimSpace(req.Country)
	req.City = strings.TrimSpace(req.City)
	req.ConfigYAML = strings.TrimSpace(req.ConfigYAML)

	if req.ConfigYAML != "" {
		if err := applyYAMLToExitRequest(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
	}

	if req.Name != "" {
		exit.Name = req.Name
	}

	if req.ConfigYAML != "" {
		if req.Protocol == "" {
			req.Protocol = "wireguard"
		}
		if req.Provider == "" {
			req.Provider = "protonvpn"
		}
		exit.Protocol = req.Protocol
		exit.Provider = req.Provider
		exit.Country = req.Country
		exit.City = req.City
	} else {
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
	}
	if req.Enabled != nil {
		exit.Enabled = *req.Enabled
	}

	if req.ConfigYAML != "" {
		configJSON, err := buildExitConfigJSON(req)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		exit.ConfigJSON = configJSON
	} else {
		creds := parseExitCredentials(exit.ConfigJSON)
		if req.WireguardPrivKey != "" {
			creds["wireguard_private_key"] = req.WireguardPrivKey
		}
		if req.WireguardPublicKey != "" {
			creds["wireguard_public_key"] = req.WireguardPublicKey
		}
		if req.WireguardEndpointIP != "" {
			creds["wireguard_endpoint_ip"] = req.WireguardEndpointIP
		}
		if req.WireguardEndpointPort != "" {
			creds["wireguard_endpoint_port"] = req.WireguardEndpointPort
		}
		if req.WireguardPresharedKey != "" {
			creds["wireguard_preshared_key"] = req.WireguardPresharedKey
		}
		if req.WireguardAllowedIPs != "" {
			creds["wireguard_allowed_ips"] = req.WireguardAllowedIPs
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
		if len(creds) > 0 {
			if raw, err := json.Marshal(creds); err == nil {
				exit.ConfigJSON = string(raw)
			}
		}
	}

	if err := validateExitConfig(&exit); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	if err := db.DB.Save(&exit).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.manager.ReconcileAfterUpdate(&before, &exit); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": fmt.Sprintf("exit updated but failed to apply runtime changes: %v", err),
		})
	}

	exit.ConfigJSON = ""
	return c.JSON(exit)
}

func (h *ExitHandler) Config(c *fiber.Ctx) error {
	exit := h.getExit(c)
	if exit == nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}

	configYAML, err := buildExitConfigYAML(exit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"config_yaml": configYAML})
}

func parseExitCredentials(raw string) map[string]string {
	result := make(map[string]string)
	if raw == "" {
		return result
	}
	_ = json.Unmarshal([]byte(raw), &result)
	return result
}

func validateExitRequest(req CreateExitRequest) error {
	if req.Protocol == "wireguard" && strings.EqualFold(req.Provider, "custom") {
		if req.WireguardEndpointIP == "" {
			return fmt.Errorf("wireguard_endpoint_ip is required for custom wireguard exits")
		}
		if req.WireguardEndpointPort == "" {
			return fmt.Errorf("wireguard_endpoint_port is required for custom wireguard exits")
		}
		if req.WireguardPublicKey == "" {
			return fmt.Errorf("wireguard_public_key is required for custom wireguard exits")
		}
		if req.WireguardPrivKey == "" {
			return fmt.Errorf("wireguard_private_key is required for custom wireguard exits")
		}
		if req.WireguardAddresses == "" {
			return fmt.Errorf("wireguard_addresses is required for custom wireguard exits")
		}
		if _, err := netip.ParseAddr(req.WireguardEndpointIP); err != nil {
			return fmt.Errorf("wireguard_endpoint_ip must be an IP address for custom wireguard exits")
		}
		port, err := strconv.Atoi(req.WireguardEndpointPort)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("wireguard_endpoint_port must be a valid port for custom wireguard exits")
		}
		hasAddress := false
		for _, address := range strings.Split(req.WireguardAddresses, ",") {
			address = strings.TrimSpace(address)
			if address == "" {
				continue
			}
			if _, err := netip.ParsePrefix(address); err != nil {
				return fmt.Errorf("wireguard_addresses must contain CIDR addresses for custom wireguard exits")
			}
			hasAddress = true
		}
		if !hasAddress {
			return fmt.Errorf("wireguard_addresses must contain CIDR addresses for custom wireguard exits")
		}
		return nil
	}
	if req.Protocol == "wireguard" && req.WireguardPrivKey == "" {
		return fmt.Errorf("wireguard_private_key is required for wireguard exits")
	}
	if req.Protocol == "openvpn" && (req.OpenVPNUser == "" || req.OpenVPNPassword == "") {
		return fmt.Errorf("openvpn_user and openvpn_password are required for openvpn exits")
	}
	return nil
}

func validateExitConfig(exit *models.Exit) error {
	raw := map[string]interface{}{}
	if exit.ConfigJSON != "" {
		_ = json.Unmarshal([]byte(exit.ConfigJSON), &raw)
	}
	env := extractStoredComposeEnv(raw)
	req := CreateExitRequest{
		Protocol:               exit.Protocol,
		Provider:               exit.Provider,
		WireguardPrivKey:       firstConfigValue(env, raw, "WIREGUARD_PRIVATE_KEY", "wireguard_private_key"),
		WireguardPublicKey:     firstConfigValue(env, raw, "WIREGUARD_PUBLIC_KEY", "wireguard_public_key"),
		WireguardEndpointIP:    firstConfigValue(env, raw, "WIREGUARD_ENDPOINT_IP", "wireguard_endpoint_ip"),
		WireguardEndpointPort:  firstConfigValue(env, raw, "WIREGUARD_ENDPOINT_PORT", "wireguard_endpoint_port"),
		WireguardPresharedKey:  firstConfigValue(env, raw, "WIREGUARD_PRESHARED_KEY", "wireguard_preshared_key"),
		WireguardAllowedIPs:    firstConfigValue(env, raw, "WIREGUARD_ALLOWED_IPS", "wireguard_allowed_ips"),
		WireguardAddresses:     firstConfigValue(env, raw, "WIREGUARD_ADDRESSES", "wireguard_addresses"),
		OpenVPNUser:            firstConfigValue(env, raw, "OPENVPN_USER", "openvpn_user"),
		OpenVPNPassword:        firstConfigValue(env, raw, "OPENVPN_PASSWORD", "openvpn_password"),
	}
	return validateExitRequest(req)
}

func firstConfigValue(env map[string]string, raw map[string]interface{}, envKey, rawKey string) string {
	if value := strings.TrimSpace(env[envKey]); value != "" {
		return value
	}
	if value, ok := raw[rawKey]; ok {
		return strings.TrimSpace(fmt.Sprint(value))
	}
	return ""
}

func applyYAMLToExitRequest(req *CreateExitRequest) error {
	raw := make(map[string]interface{})
	if err := yaml.Unmarshal([]byte(req.ConfigYAML), &raw); err != nil {
		return fmt.Errorf("invalid yaml: %w", err)
	}

	if applyComposeStyleConfig(raw, req) {
		return nil
	}

	getString := func(key string) string {
		if v, ok := raw[key]; ok {
			return strings.TrimSpace(fmt.Sprint(v))
		}
		return ""
	}

	if v := getString("runtime"); v != "" {
		req.Runtime = v
	}
	if v := getString("protocol"); v != "" {
		req.Protocol = v
	}
	if v := getString("provider"); v != "" {
		req.Provider = v
	}
	if v := getString("country"); v != "" {
		req.Country = v
	}
	if v := getString("city"); v != "" {
		req.City = v
	}
	if v := getString("wireguard_private_key"); v != "" {
		req.WireguardPrivKey = v
	}
	if v := getString("wireguard_public_key"); v != "" {
		req.WireguardPublicKey = v
	}
	if v := getString("wireguard_endpoint_ip"); v != "" {
		req.WireguardEndpointIP = v
	}
	if v := getString("wireguard_endpoint_port"); v != "" {
		req.WireguardEndpointPort = v
	}
	if v := getString("wireguard_preshared_key"); v != "" {
		req.WireguardPresharedKey = v
	}
	if v := getString("wireguard_allowed_ips"); v != "" {
		req.WireguardAllowedIPs = v
	}
	if v := getString("wireguard_addresses"); v != "" {
		req.WireguardAddresses = v
	}
	if v := getString("openvpn_user"); v != "" {
		req.OpenVPNUser = v
	}
	if v := getString("openvpn_password"); v != "" {
		req.OpenVPNPassword = v
	}
	if enabled, ok := raw["enabled"].(bool); ok {
		req.Enabled = &enabled
	}

	return nil
}

func applyComposeStyleConfig(raw map[string]interface{}, req *CreateExitRequest) bool {
	services, ok := raw["services"].(map[string]interface{})
	if !ok || len(services) == 0 {
		return false
	}

	var service map[string]interface{}
	if gluetun, ok := services["gluetun"].(map[string]interface{}); ok {
		service = gluetun
	} else {
		for _, candidate := range services {
			if asMap, ok := candidate.(map[string]interface{}); ok {
				service = asMap
				break
			}
		}
	}
	if service == nil {
		return false
	}

	req.Runtime = "gluetun"

	if enabled, ok := raw["enabled"].(bool); ok {
		req.Enabled = &enabled
	} else if enabled, ok := service["enabled"].(bool); ok {
		req.Enabled = &enabled
	}

	env := normalizeEnvironment(service["environment"])
	if len(env) == 0 {
		return true
	}

	if v := env["VPN_TYPE"]; v != "" {
		req.Protocol = v
	}
	if v := env["VPN_SERVICE_PROVIDER"]; v != "" {
		req.Provider = v
	}
	if v := env["SERVER_COUNTRIES"]; v != "" {
		req.Country = v
	}
	if v := env["SERVER_CITIES"]; v != "" {
		req.City = v
	}
	if v := env["WIREGUARD_PRIVATE_KEY"]; v != "" {
		req.WireguardPrivKey = v
	}
	if v := env["WIREGUARD_PUBLIC_KEY"]; v != "" {
		req.WireguardPublicKey = v
	}
	if v := env["WIREGUARD_ENDPOINT_IP"]; v != "" {
		req.WireguardEndpointIP = v
	}
	if v := env["WIREGUARD_ENDPOINT_PORT"]; v != "" {
		req.WireguardEndpointPort = v
	}
	if v := env["WIREGUARD_PRESHARED_KEY"]; v != "" {
		req.WireguardPresharedKey = v
	}
	if v := env["WIREGUARD_ALLOWED_IPS"]; v != "" {
		req.WireguardAllowedIPs = v
	}
	if v := env["WIREGUARD_ADDRESSES"]; v != "" {
		req.WireguardAddresses = v
	}
	if v := env["OPENVPN_USER"]; v != "" {
		req.OpenVPNUser = v
	}
	if v := env["OPENVPN_PASSWORD"]; v != "" {
		req.OpenVPNPassword = v
	}

	return true
}

func normalizeEnvironment(raw interface{}) map[string]string {
	result := make(map[string]string)

	switch env := raw.(type) {
	case map[string]interface{}:
		for key, value := range env {
			result[key] = strings.TrimSpace(fmt.Sprint(value))
		}
	case []interface{}:
		for _, item := range env {
			kv := strings.TrimSpace(fmt.Sprint(item))
			key, value, ok := strings.Cut(kv, "=")
			if ok {
				result[strings.TrimSpace(key)] = strings.TrimSpace(value)
			}
		}
	}

	return result
}

func buildExitConfigJSON(req CreateExitRequest) (string, error) {
	raw := map[string]interface{}{}
	if req.ConfigYAML != "" {
		if err := yaml.Unmarshal([]byte(req.ConfigYAML), &raw); err != nil {
			return "", fmt.Errorf("invalid yaml: %w", err)
		}
	}

	delete(raw, "name")
	if req.Runtime != "" {
		raw["runtime"] = req.Runtime
	}
	if req.Protocol != "" {
		raw["protocol"] = req.Protocol
	}
	if req.Provider != "" {
		raw["provider"] = req.Provider
	}
	if req.Country != "" {
		raw["country"] = req.Country
	}
	if req.City != "" {
		raw["city"] = req.City
	}
	if req.Enabled != nil {
		raw["enabled"] = *req.Enabled
	}
	if req.WireguardPrivKey != "" {
		raw["wireguard_private_key"] = req.WireguardPrivKey
	}
	if req.WireguardPublicKey != "" {
		raw["wireguard_public_key"] = req.WireguardPublicKey
	}
	if req.WireguardEndpointIP != "" {
		raw["wireguard_endpoint_ip"] = req.WireguardEndpointIP
	}
	if req.WireguardEndpointPort != "" {
		raw["wireguard_endpoint_port"] = req.WireguardEndpointPort
	}
	if req.WireguardPresharedKey != "" {
		raw["wireguard_preshared_key"] = req.WireguardPresharedKey
	}
	if req.WireguardAllowedIPs != "" {
		raw["wireguard_allowed_ips"] = req.WireguardAllowedIPs
	}
	if req.WireguardAddresses != "" {
		raw["wireguard_addresses"] = req.WireguardAddresses
	}
	if req.OpenVPNUser != "" {
		raw["openvpn_user"] = req.OpenVPNUser
	}
	if req.OpenVPNPassword != "" {
		raw["openvpn_password"] = req.OpenVPNPassword
	}

	encoded, err := json.Marshal(raw)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func buildExitConfigYAML(exit *models.Exit) (string, error) {
	raw := map[string]interface{}{}
	if exit.ConfigJSON != "" {
		if err := json.Unmarshal([]byte(exit.ConfigJSON), &raw); err != nil {
			return "", err
		}
	}

	envMap := extractStoredComposeEnv(raw)
	envMap["VPN_SERVICE_PROVIDER"] = exit.Provider
	envMap["VPN_TYPE"] = exit.Protocol
	envMap["SERVER_COUNTRIES"] = exit.Country
	envMap["SERVER_CITIES"] = exit.City
	copyRawString(raw, envMap, "wireguard_private_key", "WIREGUARD_PRIVATE_KEY")
	copyRawString(raw, envMap, "wireguard_public_key", "WIREGUARD_PUBLIC_KEY")
	copyRawString(raw, envMap, "wireguard_endpoint_ip", "WIREGUARD_ENDPOINT_IP")
	copyRawString(raw, envMap, "wireguard_endpoint_port", "WIREGUARD_ENDPOINT_PORT")
	copyRawString(raw, envMap, "wireguard_preshared_key", "WIREGUARD_PRESHARED_KEY")
	copyRawString(raw, envMap, "wireguard_allowed_ips", "WIREGUARD_ALLOWED_IPS")
	copyRawString(raw, envMap, "wireguard_addresses", "WIREGUARD_ADDRESSES")
	copyRawString(raw, envMap, "openvpn_user", "OPENVPN_USER")
	copyRawString(raw, envMap, "openvpn_password", "OPENVPN_PASSWORD")

	compose := map[string]interface{}{
		"enabled": exit.Enabled,
		"services": map[string]interface{}{
			"gluetun": map[string]interface{}{
				"image":       "qmcgaw/gluetun:latest",
				"environment": orderedEnvironment(envMap, exit.Protocol),
			},
		},
	}

	encoded, err := yaml.Marshal(compose)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func extractStoredComposeEnv(raw map[string]interface{}) map[string]string {
	services, ok := raw["services"].(map[string]interface{})
	if !ok {
		return map[string]string{}
	}

	var service map[string]interface{}
	if gluetun, ok := services["gluetun"].(map[string]interface{}); ok {
		service = gluetun
	} else {
		for _, candidate := range services {
			if asMap, ok := candidate.(map[string]interface{}); ok {
				service = asMap
				break
			}
		}
	}
	if service == nil {
		return map[string]string{}
	}

	return normalizeEnvironment(service["environment"])
}

func copyRawString(raw map[string]interface{}, env map[string]string, rawKey, envKey string) {
	if value, ok := raw[rawKey]; ok && fmt.Sprint(value) != "" {
		env[envKey] = fmt.Sprint(value)
	}
}

func orderedEnvironment(env map[string]string, protocol string) []string {
	order := []string{
		"VPN_SERVICE_PROVIDER",
		"VPN_TYPE",
		"SERVER_COUNTRIES",
		"SERVER_CITIES",
		"WIREGUARD_PRIVATE_KEY",
		"WIREGUARD_ADDRESSES",
	}
	if protocol == "wireguard" && strings.EqualFold(env["VPN_SERVICE_PROVIDER"], "custom") {
		order = []string{
			"VPN_SERVICE_PROVIDER",
			"VPN_TYPE",
			"WIREGUARD_ENDPOINT_IP",
			"WIREGUARD_ENDPOINT_PORT",
			"WIREGUARD_PUBLIC_KEY",
			"WIREGUARD_PRIVATE_KEY",
			"WIREGUARD_PRESHARED_KEY",
			"WIREGUARD_ADDRESSES",
			"WIREGUARD_ALLOWED_IPS",
			"WIREGUARD_MTU",
			"WIREGUARD_PERSISTENT_KEEPALIVE_INTERVAL",
		}
	}
	if protocol == "openvpn" {
		order = []string{
			"VPN_SERVICE_PROVIDER",
			"VPN_TYPE",
			"SERVER_COUNTRIES",
			"SERVER_CITIES",
			"OPENVPN_USER",
			"OPENVPN_PASSWORD",
		}
	}

	seen := make(map[string]bool, len(env))
	result := make([]string, 0, len(env))
	for _, key := range order {
		result = append(result, fmt.Sprintf("%s=%s", key, env[key]))
		seen[key] = true
	}

	extras := make([]string, 0, len(env))
	for key := range env {
		if !seen[key] {
			extras = append(extras, key)
		}
	}
	sort.Strings(extras)
	for _, key := range extras {
		result = append(result, fmt.Sprintf("%s=%s", key, env[key]))
	}

	return result
}

func (h *ExitHandler) Delete(c *fiber.Ctx) error {
	id, _ := strconv.Atoi(c.Params("id"))
	var exit models.Exit
	if err := db.DB.First(&exit, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}

	summary, err := getExitDependencySummary(db.DB, exit.ID)
	if err != nil {
		return err
	}
	if exitHasActiveDependencies(summary) {
		return c.Status(400).JSON(fiber.Map{
			"error":                       "cannot delete exit with active dependencies",
			"active_rule_count":           summary.ActiveRuleCount,
			"inactive_rule_count":         summary.InactiveRuleCount,
			"active_subscription_count":   summary.ActiveSubscriptionCount,
			"inactive_subscription_count": summary.InactiveSubscriptionCount,
		})
	}

	if err := h.manager.Delete(&exit); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	if err := db.DB.Transaction(func(tx *gorm.DB) error {
		summary, err := getExitDependencySummary(tx, exit.ID)
		if err != nil {
			return err
		}
		if exitHasActiveDependencies(summary) {
			return fiber.NewError(fiber.StatusBadRequest, "cannot delete exit with active dependencies")
		}
		if err := tx.Model(&models.Rule{}).Where("exit_id = ?", exit.ID).Update("exit_id", nil).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.Subscription{}).Where("exit_id = ?", exit.ID).Update("exit_id", nil).Error; err != nil {
			return err
		}
		if err := tx.Where("exit_id = ?", exit.ID).Delete(&models.ExitRuntimeEvent{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&exit).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

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
