package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"sni-gateway/internal/db"
	"sni-gateway/internal/docker"
	"sni-gateway/internal/models"
)

type ExitManager struct {
	operationMu sync.RWMutex
	exitMu      sync.Mutex
	exitLocks   map[uint]*sync.Mutex
	docker      *docker.Client
}

func NewExitManager(dockerClient *docker.Client) *ExitManager {
	return &ExitManager{docker: dockerClient, exitLocks: make(map[uint]*sync.Mutex)}
}

// lockExit allows independent exits to run concurrently while StopAll stays exclusive.
func (em *ExitManager) lockExit(id uint) func() {
	em.operationMu.RLock()
	unlock := em.lockExitOnly(id)
	return func() {
		unlock()
		em.operationMu.RUnlock()
	}
}

func (em *ExitManager) lockExitOnly(id uint) func() {
	lock := em.exitLock(id)
	lock.Lock()
	return lock.Unlock
}

func (em *ExitManager) exitLock(id uint) *sync.Mutex {
	em.exitMu.Lock()
	defer em.exitMu.Unlock()
	lock := em.exitLocks[id]
	if lock == nil {
		lock = &sync.Mutex{}
		em.exitLocks[id] = lock
	}
	return lock
}

func (em *ExitManager) tryLockExit(id uint) (func(), bool) {
	if !em.operationMu.TryRLock() {
		return nil, false
	}
	lock := em.exitLock(id)
	if !lock.TryLock() {
		em.operationMu.RUnlock()
		return nil, false
	}
	return func() {
		lock.Unlock()
		em.operationMu.RUnlock()
	}, true
}

func (em *ExitManager) ensureDockerAvailable() error {
	if em == nil || em.docker == nil {
		return errors.New("docker client unavailable; exit runtime management is disabled")
	}
	return nil
}

func (em *ExitManager) ReconcileAfterUpdate(before, after *models.Exit) error {
	if before == nil || after == nil {
		return nil
	}
	if after.Runtime == "sing-box" {
		return em.reconcileSingBox(before, after)
	}
	if after.Runtime == "gluetun" {
		return em.reconcileProvider(before, after)
	}
	defer em.lockExit(after.ID)()

	runtimeChanged := exitRuntimeConfigChanged(before, after)
	if !runtimeChanged && before.Enabled == after.Enabled && (after.Enabled || before.Status == "stopped" || before.Status == "not_found") {
		return nil
	}

	if err := em.ensureDockerAvailable(); err != nil {
		em.updateStatus(after, "unhealthy")
		em.logEvent(after.ID, "update", "failed", err.Error())
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	em.logEvent(after.ID, "update", "started", "reconciling runtime")

	if runtimeChanged || !after.Enabled {
		var failures []error
		for _, name := range changedRuntimeNames(before.Name, after.Name) {
			if err := removeExitContainers(ctx, em.docker, name); err != nil {
				failures = append(failures, err)
			}
		}
		if err := errors.Join(failures...); err != nil {
			em.updateStatus(after, "unhealthy")
			em.logEvent(after.ID, "update", "failed", err.Error())
			return err
		}
	}
	if !after.Enabled {
		em.updateStatus(after, "stopped")
		em.logEvent(after.ID, "update", "success", "VPN and relay disabled")
		return nil
	}

	if runtimeChanged && before.Enabled == after.Enabled && before.Status == "stopped" {
		em.updateStatus(after, "stopped")
		em.logEvent(after.ID, "update", "success", "runtime config saved; will recreate on next start")
		return nil
	}

	if !before.Enabled || runtimeChanged {
		if err := em.createAndStart(after); err != nil {
			em.logEvent(after.ID, "update", "failed", err.Error())
			return err
		}
		em.logEvent(after.ID, "update", "success", "runtime recreated")
	}

	return nil
}

func (em *ExitManager) createAndStart(exit *models.Exit) error {
	if exit.Runtime == "sing-box" {
		return em.createAndStartSingBox(exit)
	}
	if exit.Runtime == "gluetun" {
		return em.createAndStartProvider(exit)
	}
	if err := em.ensureDockerAvailable(); err != nil {
		em.updateStatus(exit, "unhealthy")
		em.logEvent(exit.ID, "create", "failed", err.Error())
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	em.logEvent(exit.ID, "create", "started", "")

	gluetunCfg := docker.GluetunConfig{
		Name:      exit.Name,
		Protocol:  exit.Protocol,
		Provider:  exit.Provider,
		Country:   exit.Country,
		City:      exit.City,
		RelayPort: 10000 + int(exit.ID),
	}

	if exit.ConfigJSON != "" {
		cfg := parseConfigJSON(exit.ConfigJSON)
		gluetunCfg.ExtraEnv = extractStoredGluetunEnv(cfg)
		copyRawConfigToEnv(cfg, gluetunCfg.ExtraEnv, "wireguard_public_key", "WIREGUARD_PUBLIC_KEY")
		copyRawConfigToEnv(cfg, gluetunCfg.ExtraEnv, "wireguard_endpoint_ip", "WIREGUARD_ENDPOINT_IP")
		copyRawConfigToEnv(cfg, gluetunCfg.ExtraEnv, "wireguard_endpoint_port", "WIREGUARD_ENDPOINT_PORT")
		copyRawConfigToEnv(cfg, gluetunCfg.ExtraEnv, "wireguard_preshared_key", "WIREGUARD_PRESHARED_KEY")
		copyRawConfigToEnv(cfg, gluetunCfg.ExtraEnv, "wireguard_allowed_ips", "WIREGUARD_ALLOWED_IPS")
		copyRawConfigToEnv(cfg, gluetunCfg.ExtraEnv, "wireguard_dns", "WIREGUARD_DNS")
		if key, ok := cfg["wireguard_private_key"].(string); ok {
			gluetunCfg.WireguardKey = key
		}
		if key, ok := cfg["wireguard_public_key"].(string); ok {
			gluetunCfg.WireguardPubKey = key
		}
		if addr, ok := cfg["wireguard_addresses"].(string); ok {
			gluetunCfg.WireguardAddr = addr
		}
		if user, ok := cfg["openvpn_user"].(string); ok {
			gluetunCfg.OpenVPNUser = user
		}
		if pass, ok := cfg["openvpn_password"].(string); ok {
			gluetunCfg.OpenVPNPass = pass
		}
	}

	if err := em.docker.CreateGluetunContainer(ctx, gluetunCfg); err != nil {
		_ = em.docker.RemoveContainer(ctx, "gluetun-"+exit.Name, true)
		em.updateStatus(exit, "unhealthy")
		em.logEvent(exit.ID, "create", "failed", err.Error())
		return fmt.Errorf("create gluetun: %w", err)
	}

	relayCfg := docker.RelayConfig{
		Name:       exit.Name,
		ListenPort: 10000 + int(exit.ID),
	}
	if err := em.docker.CreateRelayContainer(ctx, relayCfg); err != nil {
		_ = em.docker.RemoveContainer(ctx, "gluetun-"+exit.Name, true)
		em.updateStatus(exit, "unhealthy")
		em.logEvent(exit.ID, "create", "failed", err.Error())
		return fmt.Errorf("create relay: %w", err)
	}

	em.updateStatus(exit, "running")
	em.logEvent(exit.ID, "create", "success", "containers created")

	go em.refreshPublicIP(exit.ID, exit.Name)

	return nil
}

func (em *ExitManager) start(exit *models.Exit) error {
	if exit.Runtime == "sing-box" {
		return em.createAndStartSingBox(exit)
	}
	if exit.Runtime == "gluetun" {
		return em.restartProvider(exit)
	}
	if err := em.ensureDockerAvailable(); err != nil {
		em.logEvent(exit.ID, "start", "failed", err.Error())
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	em.logEvent(exit.ID, "start", "started", "")

	gluetunName := "gluetun-" + exit.Name
	relayName := "relay-" + exit.Name
	relayPort := 10000 + int(exit.ID)

	needsUpgrade, err := em.docker.GluetunNeedsUpgrade(ctx, exit.Name, relayPort)
	if err != nil {
		em.logEvent(exit.ID, "start", "failed", err.Error())
		return err
	}
	if needsUpgrade {
		_ = em.docker.RemoveContainer(ctx, relayName, true)
		_ = em.docker.RemoveContainer(ctx, gluetunName, true)
		if err := em.createAndStart(exit); err != nil {
			em.logEvent(exit.ID, "start", "failed", err.Error())
			return err
		}
		em.logEvent(exit.ID, "start", "success", "runtime upgraded")
		return nil
	}

	gStatus, err := em.docker.GetContainerStatus(ctx, gluetunName)
	if err != nil {
		em.logEvent(exit.ID, "start", "failed", err.Error())
		return err
	}
	if gStatus == "not_found" {
		if err := em.createAndStart(exit); err != nil {
			em.logEvent(exit.ID, "start", "failed", err.Error())
			return err
		}
		em.logEvent(exit.ID, "start", "success", "runtime recreated")
		return nil
	}

	if gStatus != "running" && gStatus != "restarting" {
		if err := em.docker.StartContainer(ctx, gluetunName); err != nil {
			em.logEvent(exit.ID, "start", "failed", err.Error())
			return err
		}
	}
	if err := em.docker.WaitForContainerRunning(ctx, gluetunName); err != nil {
		em.logEvent(exit.ID, "start", "failed", err.Error())
		return err
	}

	rStatus, err := em.docker.GetContainerStatus(ctx, relayName)
	if err != nil {
		em.logEvent(exit.ID, "start", "failed", err.Error())
		return err
	}
	if rStatus == "not_found" {
		relayCfg := docker.RelayConfig{
			Name:       exit.Name,
			ListenPort: relayPort,
		}
		if err := em.docker.CreateRelayContainer(ctx, relayCfg); err != nil {
			em.logEvent(exit.ID, "start", "failed", err.Error())
			return err
		}
	} else if err := em.docker.StartContainer(ctx, relayName); err != nil {
		em.logEvent(exit.ID, "start", "failed", err.Error())
		return err
	}

	em.updateStatus(exit, "running")
	em.logEvent(exit.ID, "start", "success", "")
	return nil
}

func (em *ExitManager) Stop(exit *models.Exit) error {
	defer em.lockExit(exit.ID)()
	return em.stopWithContext(context.Background(), exit)
}

func (em *ExitManager) restart(exit *models.Exit) error {
	if exit.Runtime == "sing-box" {
		return em.createAndStartSingBox(exit)
	}
	if exit.Runtime == "gluetun" {
		return em.restartProvider(exit)
	}
	if err := em.ensureDockerAvailable(); err != nil {
		em.logEvent(exit.ID, "restart", "failed", err.Error())
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	em.logEvent(exit.ID, "restart", "started", "")

	relayPort := 10000 + int(exit.ID)
	needsUpgrade, err := em.docker.GluetunNeedsUpgrade(ctx, exit.Name, relayPort)
	if err != nil {
		em.logEvent(exit.ID, "restart", "failed", err.Error())
		return err
	}
	if needsUpgrade {
		_ = em.docker.RemoveContainer(ctx, "relay-"+exit.Name, true)
		_ = em.docker.RemoveContainer(ctx, "gluetun-"+exit.Name, true)
		if err := em.createAndStart(exit); err != nil {
			em.logEvent(exit.ID, "restart", "failed", err.Error())
			return err
		}
		em.logEvent(exit.ID, "restart", "success", "runtime upgraded")
		return nil
	}

	gStatus, err := em.docker.GetContainerStatus(ctx, "gluetun-"+exit.Name)
	if err != nil {
		em.logEvent(exit.ID, "restart", "failed", err.Error())
		return err
	}
	if gStatus == "not_found" {
		if err := em.createAndStart(exit); err != nil {
			em.logEvent(exit.ID, "restart", "failed", err.Error())
			return err
		}
		em.logEvent(exit.ID, "restart", "success", "runtime recreated")
		return nil
	}

	if gStatus != "restarting" {
		if err := em.docker.RestartContainer(ctx, "gluetun-"+exit.Name); err != nil {
			em.logEvent(exit.ID, "restart", "failed", err.Error())
			return err
		}
	}
	if err := em.docker.WaitForContainerRunning(ctx, "gluetun-"+exit.Name); err != nil {
		em.logEvent(exit.ID, "restart", "failed", err.Error())
		return err
	}

	rStatus, err := em.docker.GetContainerStatus(ctx, "relay-"+exit.Name)
	if err != nil {
		em.logEvent(exit.ID, "restart", "failed", err.Error())
		return err
	}
	if rStatus == "not_found" {
		relayCfg := docker.RelayConfig{
			Name:       exit.Name,
			ListenPort: relayPort,
		}
		if err := em.docker.CreateRelayContainer(ctx, relayCfg); err != nil {
			em.logEvent(exit.ID, "restart", "failed", err.Error())
			return err
		}
	} else if err := em.docker.RestartContainer(ctx, "relay-"+exit.Name); err != nil {
		em.logEvent(exit.ID, "restart", "failed", err.Error())
		return err
	}

	em.updateStatus(exit, "running")
	em.logEvent(exit.ID, "restart", "success", "")
	return nil
}

func (em *ExitManager) RestartEnabledExits() error {
	em.operationMu.RLock()
	defer em.operationMu.RUnlock()
	var exits []models.Exit
	if err := db.DB.Where("enabled = true AND status = ?", "running").Order("id ASC").Find(&exits).Error; err != nil {
		return err
	}
	if len(exits) == 0 {
		log.Println("Enabled exit restart skipped: no enabled exits")
		return nil
	}

	failures := make([]string, 0)
	for i := range exits {
		exit := exits[i]
		unlock := em.lockExitOnly(exit.ID)
		var current models.Exit
		err := db.DB.First(&current, exit.ID).Error
		if err == nil && current.Enabled && current.Status == "running" {
			err = em.restart(&current)
		}
		unlock()
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", exit.Name, err))
			continue
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("failed to restart %d of %d exits: %s", len(failures), len(exits), strings.Join(failures, "; "))
	}
	return nil
}

func (em *ExitManager) delete(exit *models.Exit) error {
	if exit.Runtime == "sing-box" {
		return em.deleteSingBox(exit)
	}
	if exit.Runtime == "gluetun" {
		if err := em.ensureDockerAvailable(); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		return em.removeProviderContainers(ctx, exit.Name)
	}
	if err := em.ensureDockerAvailable(); err != nil {
		em.logEvent(exit.ID, "delete", "failed", err.Error())
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	em.logEvent(exit.ID, "delete", "started", "")

	if err := removeExitContainers(ctx, em.docker, exit.Name); err != nil {
		em.logEvent(exit.ID, "delete", "failed", err.Error())
		return err
	}

	em.logEvent(exit.ID, "delete", "success", "containers removed")
	return nil
}

func (em *ExitManager) GetStatus(exit *models.Exit) (string, string, error) {
	defer em.lockExit(exit.ID)()
	if exit.Runtime == "sing-box" {
		return em.getSingBoxStatus(exit)
	}
	if exit.Runtime == "gluetun" {
		return em.getProviderStatus(exit)
	}
	if err := em.ensureDockerAvailable(); err != nil {
		return "unavailable", "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	gStatus, err := em.docker.GetContainerStatus(ctx, "gluetun-"+exit.Name)
	if err != nil {
		return "unknown", "", err
	}

	status := "stopped"
	ip := ""
	switch gStatus {
	case "running":
		relayStatus, err := em.docker.GetContainerStatus(ctx, "relay-"+exit.Name)
		if err != nil {
			return "unknown", "", err
		}
		status = "unhealthy"
		if relayStatus == "running" {
			var ipErr error
			ip, ipErr = em.docker.GetPublicIP(ctx, exit.Name)
			if ipErr == nil {
				status = "running"
			} else {
				log.Printf("Exit %s public IP probe failed: %v", exit.Name, ipErr)
			}
		}
	case "not_found":
		status = "not_found"
	case "exited", "dead":
		status = "stopped"
	default:
		status = gStatus
	}

	if err := db.DB.Model(&models.Exit{}).Where("id = ?", exit.ID).Updates(map[string]interface{}{"status": status, "public_ip": ip}).Error; err != nil {
		return status, ip, err
	}
	return status, ip, nil
}

func (em *ExitManager) GetLogs(exit *models.Exit, tail int) (string, error) {
	if exit.Runtime == "sing-box" {
		return em.getSingBoxLogs(exit, tail)
	}
	if err := em.ensureDockerAvailable(); err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return em.docker.GetContainerLogs(ctx, "gluetun-"+exit.Name, tail)
}

func (em *ExitManager) updateStatus(exit *models.Exit, status string) {
	exit.Status = status
	if err := db.DB.Model(&models.Exit{}).Where("id = ?", exit.ID).Update("status", status).Error; err != nil {
		log.Printf("Failed to update exit %d status: %v", exit.ID, err)
	}
}

func (em *ExitManager) logEvent(exitID uint, action, status, msg string) {
	em.logEventContext(context.Background(), exitID, action, status, msg)
}

func (em *ExitManager) logEventContext(ctx context.Context, exitID uint, action, status, msg string) {
	event := models.ExitRuntimeEvent{
		ExitID:  exitID,
		Action:  action,
		Status:  status,
		Message: msg,
	}
	db.DB.WithContext(ctx).Create(&event)
}

func (em *ExitManager) refreshPublicIP(exitID uint, exitName string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ip, err := em.docker.GetPublicIP(ctx, exitName)
	if err != nil {
		log.Printf("Could not get public IP for %s: %v", exitName, err)
		return
	}
	db.DB.Model(&models.Exit{}).Where("id = ? AND status = ?", exitID, "running").Update("public_ip", ip)
}

func parseConfigJSON(raw string) map[string]interface{} {
	result := make(map[string]interface{})
	json.Unmarshal([]byte(raw), &result)
	return result
}

func copyRawConfigToEnv(raw map[string]interface{}, env map[string]string, rawKey, envKey string) {
	if env == nil {
		return
	}
	if value, ok := raw[rawKey]; ok && strings.TrimSpace(fmt.Sprint(value)) != "" {
		env[envKey] = strings.TrimSpace(fmt.Sprint(value))
	}
}

func extractStoredGluetunEnv(raw map[string]interface{}) map[string]string {
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

	return normalizeStoredEnv(service["environment"])
}

func normalizeStoredEnv(raw interface{}) map[string]string {
	result := make(map[string]string)

	switch env := raw.(type) {
	case map[string]interface{}:
		for key, value := range env {
			result[strings.TrimSpace(key)] = strings.TrimSpace(fmt.Sprint(value))
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

func exitRuntimeConfigChanged(before, after *models.Exit) bool {
	if before.Name != after.Name {
		return true
	}
	if before.Runtime != after.Runtime {
		return true
	}
	if before.Protocol != after.Protocol {
		return true
	}
	if before.Provider != after.Provider {
		return true
	}
	if before.Country != after.Country && (before.Runtime != "sing-box" || after.Runtime != "sing-box" || before.City == "" || after.City == "") {
		return true
	}
	if before.City != after.City {
		return true
	}
	return before.ConfigJSON != after.ConfigJSON
}

func changedRuntimeNames(beforeName, afterName string) []string {
	if beforeName == afterName {
		return []string{afterName}
	}
	return []string{beforeName, afterName}
}

func (em *ExitManager) CreateAndStart(exit *models.Exit) error {
	if exit.Runtime != "sing-box" && exit.Runtime != "gluetun" {
		return errors.New("legacy exit: edit it with a complete WireGuard configuration before starting")
	}
	defer em.lockExit(exit.ID)()
	return em.createAndStart(exit)
}

func (em *ExitManager) Start(exit *models.Exit) error {
	if exit.Runtime != "sing-box" && exit.Runtime != "gluetun" {
		return errors.New("legacy exit: edit it with a complete WireGuard configuration before starting")
	}
	defer em.lockExit(exit.ID)()
	return em.start(exit)
}

func (em *ExitManager) Restart(exit *models.Exit) error {
	if exit.Runtime != "sing-box" && exit.Runtime != "gluetun" {
		return errors.New("legacy exit: edit it with a complete WireGuard configuration before starting")
	}
	defer em.lockExit(exit.ID)()
	return em.restart(exit)
}

func (em *ExitManager) Delete(exit *models.Exit) error {
	defer em.lockExit(exit.ID)()
	return em.delete(exit)
}
