package services

import (
	"context"
	"encoding/json"
	"errors"
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

func (em *ExitManager) ensureDockerAvailable() error {
	if em == nil || em.docker == nil {
		return errors.New("docker client unavailable; exit runtime management is disabled")
	}
	return nil
}

func (em *ExitManager) CreateAndStart(exit *models.Exit) error {
	if err := em.ensureDockerAvailable(); err != nil {
		em.updateStatus(exit, "unhealthy")
		em.logEvent(exit.ID, "create", "failed", err.Error())
		return err
	}

	ctx := context.Background()
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
		_ = em.docker.RemoveContainer(ctx, "gluetun-"+exit.Name, true)
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
	if err := em.ensureDockerAvailable(); err != nil {
		em.logEvent(exit.ID, "start", "failed", err.Error())
		return err
	}

	ctx := context.Background()
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
		if err := em.CreateAndStart(exit); err != nil {
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
		if err := em.CreateAndStart(exit); err != nil {
			em.logEvent(exit.ID, "start", "failed", err.Error())
			return err
		}
		em.logEvent(exit.ID, "start", "success", "runtime recreated")
		return nil
	}

	if err := em.docker.StartContainer(ctx, gluetunName); err != nil {
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
	if err := em.ensureDockerAvailable(); err != nil {
		em.logEvent(exit.ID, "stop", "failed", err.Error())
		return err
	}

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
	if err := em.ensureDockerAvailable(); err != nil {
		em.logEvent(exit.ID, "restart", "failed", err.Error())
		return err
	}

	ctx := context.Background()
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
		if err := em.CreateAndStart(exit); err != nil {
			em.logEvent(exit.ID, "restart", "failed", err.Error())
			return err
		}
		em.logEvent(exit.ID, "restart", "success", "runtime upgraded")
		return nil
	}

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
	if err := em.ensureDockerAvailable(); err != nil {
		em.logEvent(exit.ID, "delete", "failed", err.Error())
		return err
	}

	ctx := context.Background()
	em.logEvent(exit.ID, "delete", "started", "")

	_ = em.docker.RemoveContainer(ctx, "relay-"+exit.Name, true)
	_ = em.docker.RemoveContainer(ctx, "gluetun-"+exit.Name, true)

	em.logEvent(exit.ID, "delete", "success", "containers removed")
	return nil
}

func (em *ExitManager) GetStatus(exit *models.Exit) (string, string, error) {
	if err := em.ensureDockerAvailable(); err != nil {
		return "unavailable", "", err
	}

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
	if err := em.ensureDockerAvailable(); err != nil {
		return "", err
	}

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
