package services

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"sni-gateway/internal/db"
	"sni-gateway/internal/gateway"
	"sni-gateway/internal/models"
	"sni-gateway/internal/singbox"
)

func (em *ExitManager) createAndStartSingBox(exit *models.Exit) error {
	if err := em.ensureDockerAvailable(); err != nil {
		return err
	}
	config, err := singbox.BuildExitConfig(*exit)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	em.logEvent(exit.ID, "start", "started", "starting sing-box")
	if err := em.docker.CreateSingBoxContainer(ctx, exit.Name, config); err != nil {
		em.updateStatus(exit, "unhealthy")
		em.logEvent(exit.ID, "start", "failed", err.Error())
		return err
	}
	em.updateStatus(exit, "running")
	em.logEvent(exit.ID, "start", "success", "sing-box started")
	go em.refreshSingBoxPublicIP(exit.ID, exit.Name)
	return nil
}

func (em *ExitManager) reconcileSingBox(before, after *models.Exit) error {
	defer em.lockExit(after.ID)()
	if !exitRuntimeConfigChanged(before, after) && before.Enabled == after.Enabled {
		return nil
	}
	if err := em.ensureDockerAvailable(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if before.Runtime != "sing-box" {
		if err := removeExitContainers(ctx, em.docker, before.Name); err != nil {
			return err
		}
	}
	if before.Name != after.Name || before.Runtime != "sing-box" {
		if err := em.docker.RemoveContainer(ctx, "singbox-"+before.Name, true); err != nil {
			return err
		}
	}
	if !after.Enabled {
		if err := em.docker.RemoveContainer(ctx, "singbox-"+after.Name, true); err != nil {
			return err
		}
		em.updateStatus(after, "stopped")
		return nil
	}
	return em.createAndStartSingBox(after)
}

func (em *ExitManager) stopSingBox(ctx context.Context, exit *models.Exit) error {
	if err := em.ensureDockerAvailable(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if err := em.docker.StopContainer(ctx, "singbox-"+exit.Name); err != nil {
		return err
	}
	exit.Status, exit.PublicIP = "stopped", ""
	return db.DB.Model(exit).Updates(map[string]any{"status": "stopped", "public_ip": ""}).Error
}

func (em *ExitManager) deleteSingBox(exit *models.Exit) error {
	if err := em.ensureDockerAvailable(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	return em.docker.RemoveContainer(ctx, "singbox-"+exit.Name, true)
}

func (em *ExitManager) getSingBoxLogs(exit *models.Exit, tail int) (string, error) {
	if err := em.ensureDockerAvailable(); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return em.docker.GetContainerLogs(ctx, "singbox-"+exit.Name, tail)
}

func (em *ExitManager) getSingBoxStatus(exit *models.Exit) (string, string, error) {
	if err := em.ensureDockerAvailable(); err != nil {
		return "unavailable", "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	status, err := em.docker.GetContainerStatus(ctx, "singbox-"+exit.Name)
	if err != nil {
		return "unknown", "", err
	}
	ip := ""
	switch status {
	case "running":
		ip, err = probeSingBoxPublicIP(exit.Name)
		if err != nil {
			status = "unhealthy"
		}
	case "exited", "dead":
		status = "stopped"
	}
	if err := db.DB.Model(exit).Updates(map[string]any{"status": status, "public_ip": ip}).Error; err != nil {
		return status, ip, err
	}
	return status, ip, nil
}

func (em *ExitManager) refreshSingBoxPublicIP(id uint, name string) {
	ip, err := probeSingBoxPublicIP(name)
	if err != nil {
		return
	}
	db.DB.Model(&models.Exit{}).Where("id = ? AND status = ?", id, "running").Update("public_ip", ip)
}

func probeSingBoxPublicIP(name string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return gateway.DialSOCKS("singbox-"+name+":1080", "api.ipify.org", 443)
	}}}
	response, err := client.Get("https://api.ipify.org")
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("public IP probe returned %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 128))
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(string(data))
	if _, err := netip.ParseAddr(ip); err != nil {
		return "", fmt.Errorf("invalid public IP response")
	}
	return ip, nil
}
