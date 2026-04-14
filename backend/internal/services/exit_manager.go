package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"sni-gateway/internal/db"
	"sni-gateway/internal/docker"
	"sni-gateway/internal/models"
)

type ExitManager struct {
	docker *docker.Client
}

func NewExitManager(dockerClient *docker.Client) *ExitManager {
	return &ExitManager{docker: dockerClient}
}

func (em *ExitManager) CreateAndStart(exit *models.Exit) error {
	ctx := context.Background()
	em.logEvent(exit.ID, "create", "started", "")

	gluetunCfg := docker.GluetunConfig{
		Name:     exit.Name,
		Protocol: exit.Protocol,
		Provider: exit.Provider,
		Country:  exit.Country,
		City:     exit.City,
	}

	if exit.ConfigJSON != "" {
		cfg := parseConfigJSON(exit.ConfigJSON)
		if key, ok := cfg["wireguard_private_key"].(string); ok {
			gluetunCfg.WireguardKey = key
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
		em.updateStatus(exit, "unhealthy")
		em.logEvent(exit.ID, "create", "failed", err.Error())
		return fmt.Errorf("create gluetun: %w", err)
	}

	relayCfg := docker.RelayConfig{
		Name:       exit.Name,
		ListenPort: 10000 + int(exit.ID),
	}
	if err := em.docker.CreateRelayContainer(ctx, relayCfg); err != nil {
		em.updateStatus(exit, "unhealthy")
		em.logEvent(exit.ID, "create", "failed", err.Error())
		return fmt.Errorf("create relay: %w", err)
	}

	em.updateStatus(exit, "running")
	em.logEvent(exit.ID, "create", "success", "containers created")

	ip, err := em.docker.GetPublicIP(ctx, exit.Name)
	if err != nil {
		log.Printf("Could not get public IP for %s: %v", exit.Name, err)
	} else {
		exit.PublicIP = ip
		db.DB.Save(exit)
	}

	return nil
}

func (em *ExitManager) Start(exit *models.Exit) error {
	ctx := context.Background()
	em.logEvent(exit.ID, "start", "started", "")

	if err := em.docker.StartContainer(ctx, "gluetun-"+exit.Name); err != nil {
		em.logEvent(exit.ID, "start", "failed", err.Error())
		return err
	}
	if err := em.docker.StartContainer(ctx, "relay-"+exit.Name); err != nil {
		em.logEvent(exit.ID, "start", "failed", err.Error())
		return err
	}

	em.updateStatus(exit, "running")
	em.logEvent(exit.ID, "start", "success", "")
	return nil
}

func (em *ExitManager) Stop(exit *models.Exit) error {
	ctx := context.Background()
	em.logEvent(exit.ID, "stop", "started", "")

	_ = em.docker.StopContainer(ctx, "relay-"+exit.Name)
	if err := em.docker.StopContainer(ctx, "gluetun-"+exit.Name); err != nil {
		em.logEvent(exit.ID, "stop", "failed", err.Error())
		return err
	}

	em.updateStatus(exit, "stopped")
	em.logEvent(exit.ID, "stop", "success", "")
	return nil
}

func (em *ExitManager) Restart(exit *models.Exit) error {
	ctx := context.Background()
	em.logEvent(exit.ID, "restart", "started", "")

	_ = em.docker.RestartContainer(ctx, "relay-"+exit.Name)
	if err := em.docker.RestartContainer(ctx, "gluetun-"+exit.Name); err != nil {
		em.logEvent(exit.ID, "restart", "failed", err.Error())
		return err
	}

	em.updateStatus(exit, "running")
	em.logEvent(exit.ID, "restart", "success", "")
	return nil
}

func (em *ExitManager) Delete(exit *models.Exit) error {
	ctx := context.Background()
	em.logEvent(exit.ID, "delete", "started", "")

	_ = em.docker.RemoveContainer(ctx, "relay-"+exit.Name, true)
	_ = em.docker.RemoveContainer(ctx, "gluetun-"+exit.Name, true)

	em.logEvent(exit.ID, "delete", "success", "containers removed")
	return nil
}

func (em *ExitManager) GetStatus(exit *models.Exit) (string, string, error) {
	ctx := context.Background()

	gStatus, err := em.docker.GetContainerStatus(ctx, "gluetun-"+exit.Name)
	if err != nil {
		return "unknown", "", err
	}

	ip, _ := em.docker.GetPublicIP(ctx, exit.Name)

	status := "stopped"
	switch gStatus {
	case "running":
		status = "running"
	case "not_found":
		status = "not_found"
	case "exited", "dead":
		status = "stopped"
	default:
		status = gStatus
	}

	return status, ip, nil
}

func (em *ExitManager) GetLogs(exit *models.Exit, tail int) (string, error) {
	ctx := context.Background()
	return em.docker.GetContainerLogs(ctx, "gluetun-"+exit.Name, tail)
}

func (em *ExitManager) updateStatus(exit *models.Exit, status string) {
	exit.Status = status
	db.DB.Save(exit)
}

func (em *ExitManager) logEvent(exitID uint, action, status, msg string) {
	event := models.ExitRuntimeEvent{
		ExitID:  exitID,
		Action:  action,
		Status:  status,
		Message: msg,
	}
	db.DB.Create(&event)
}

func parseConfigJSON(raw string) map[string]interface{} {
	result := make(map[string]interface{})
	json.Unmarshal([]byte(raw), &result)
	return result
}
