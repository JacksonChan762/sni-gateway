package services

import (
	"context"
	"fmt"
	"time"

	"sni-gateway/internal/db"
	"sni-gateway/internal/docker"
	"sni-gateway/internal/models"
	"sni-gateway/internal/provider"
	"sni-gateway/internal/singbox"
)

func (em *ExitManager) createAndStartProvider(exit *models.Exit) error {
	if err := em.ensureDockerAvailable(); err != nil {
		return err
	}
	env, err := provider.Env(*exit)
	if err != nil {
		return err
	}
	proxy, err := singbox.BuildGluetunProxyConfig()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	em.logEvent(exit.ID, "start", "started", "starting Gluetun provider and sing-box proxy")
	cfg := docker.GluetunConfig{Name: exit.Name, Protocol: exit.Protocol, Provider: exit.Provider, Country: exit.Country, City: exit.City, ExtraEnv: env, ProxyAlias: true}
	if err := em.docker.CreateGluetunContainer(ctx, cfg); err != nil {
		_ = em.docker.RemoveContainer(ctx, "gluetun-"+exit.Name, true)
		em.updateStatus(exit, "unhealthy")
		em.logEvent(exit.ID, "start", "failed", err.Error())
		return err
	}
	if err := em.docker.CreateSingBoxContainer(ctx, exit.Name, proxy, "container:gluetun-"+exit.Name); err != nil {
		_ = em.docker.RemoveContainer(ctx, "gluetun-"+exit.Name, true)
		em.updateStatus(exit, "unhealthy")
		em.logEvent(exit.ID, "start", "failed", err.Error())
		return err
	}
	ip, err := probeProviderPublicIP(ctx, exit.Name)
	if err != nil {
		exit.Status, exit.PublicIP = "unhealthy", ""
		if dbErr := db.DB.Model(exit).Updates(map[string]any{"status": exit.Status, "public_ip": ""}).Error; dbErr != nil {
			return dbErr
		}
		em.logEvent(exit.ID, "start", "failed", "provider started but VPN probe failed: "+err.Error())
		return nil
	}
	exit.Status, exit.PublicIP = "running", ip
	if err := db.DB.Model(exit).Updates(map[string]any{"status": exit.Status, "public_ip": ip}).Error; err != nil {
		return err
	}
	em.logEvent(exit.ID, "start", "success", "VPN connected; public IP "+ip)
	return nil
}

// Provider tunnels may need time to establish DNS and routes before the
// public-IP probe can succeed.
func probeProviderPublicIP(ctx context.Context, name string) (string, error) {
	return probeProviderPublicIPFor(ctx, name, 90*time.Second)
}

func probeProviderPublicIPFor(ctx context.Context, name string, timeout time.Duration) (string, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	var lastErr error
	for {
		ip, err := probeSingBoxPublicIP(name)
		if err == nil {
			return ip, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("VPN probe stopped: %w", ctx.Err())
		case <-deadline.C:
			return "", fmt.Errorf("VPN probe did not succeed within %s: %w", timeout, lastErr)
		case <-time.After(3 * time.Second):
		}
	}
}

func (em *ExitManager) removeProviderContainers(ctx context.Context, name string) error {
	if err := em.docker.RemoveContainer(ctx, "singbox-"+name, true); err != nil {
		return err
	}
	return em.docker.RemoveContainer(ctx, "gluetun-"+name, true)
}

func (em *ExitManager) reconcileProvider(before, after *models.Exit) error {
	defer em.lockExit(after.ID)()
	if !exitRuntimeConfigChanged(before, after) && before.Enabled == after.Enabled {
		return nil
	}
	if err := em.ensureDockerAvailable(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if before.Runtime == "gluetun" {
		if err := em.removeProviderContainers(ctx, before.Name); err != nil {
			return err
		}
	} else if before.Runtime == "sing-box" {
		if err := em.docker.RemoveContainer(ctx, "singbox-"+before.Name, true); err != nil {
			return err
		}
	} else {
		if err := removeExitContainers(ctx, em.docker, before.Name); err != nil {
			return err
		}
	}
	if !after.Enabled {
		em.updateStatus(after, "stopped")
		return nil
	}
	return em.createAndStartProvider(after)
}

func (em *ExitManager) stopProvider(ctx context.Context, exit *models.Exit) error {
	if err := em.ensureDockerAvailable(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if err := em.docker.StopContainer(ctx, "singbox-"+exit.Name); err != nil {
		return err
	}
	if err := em.docker.StopContainer(ctx, "gluetun-"+exit.Name); err != nil {
		return err
	}
	exit.Status, exit.PublicIP = "stopped", ""
	return db.DB.Model(exit).Updates(map[string]any{"status": "stopped", "public_ip": ""}).Error
}

func (em *ExitManager) getProviderStatus(exit *models.Exit) (string, string, error) {
	if err := em.ensureDockerAvailable(); err != nil {
		return "unavailable", "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	vpnStatus, err := em.docker.GetContainerStatus(ctx, "gluetun-"+exit.Name)
	if err != nil {
		return "unknown", "", err
	}
	proxyStatus, err := em.docker.GetContainerStatus(ctx, "singbox-"+exit.Name)
	if err != nil {
		return "unknown", "", err
	}
	status, ip := "stopped", ""
	if vpnStatus == "not_found" || proxyStatus == "not_found" {
		status = "not_found"
	} else if vpnStatus == "running" && proxyStatus == "running" {
		ip, err = probeSingBoxPublicIP(exit.Name)
		if err != nil {
			status = "unhealthy"
			ip = ""
		} else {
			status = "running"
		}
	} else if vpnStatus == "restarting" || proxyStatus == "restarting" {
		status = "unhealthy"
	}
	if err := db.DB.Model(exit).Updates(map[string]any{"status": status, "public_ip": ip}).Error; err != nil {
		return status, ip, err
	}
	return status, ip, nil
}

func (em *ExitManager) restartProvider(exit *models.Exit) error {
	if err := em.ensureDockerAvailable(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := em.removeProviderContainers(ctx, exit.Name); err != nil {
		return fmt.Errorf("remove old provider containers: %w", err)
	}
	return em.createAndStartProvider(exit)
}
