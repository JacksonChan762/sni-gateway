package services

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"net"
	"net/netip"
	"slices"
	"strings"
	"time"

	"sni-gateway/internal/db"
	"sni-gateway/internal/models"
)

const (
	ddnsObservationWindow = 2 * time.Minute
	ddnsRestartCooldown   = 5 * time.Minute
)

type ddnsObservation struct {
	name         string
	config       [sha256.Size]byte
	baseline     []netip.Addr
	candidate    []netip.Addr
	changedAt    time.Time
	retryAfter   time.Time
	failedProbes int
	restarted    bool
}

func ddnsHostname(exit models.Exit) string {
	if !exit.Enabled || exit.Runtime != "sing-box" || exit.Protocol != "wireguard" || exit.Provider != "custom" ||
		(exit.Status != "running" && exit.Status != "unhealthy") {
		return ""
	}
	raw := parseConfigJSON(exit.ConfigJSON)
	host, _ := raw["wireguard_endpoint_ip"].(string)
	host = strings.TrimSpace(host)
	if _, err := netip.ParseAddr(host); err == nil {
		return ""
	}
	return host
}

func (s *ddnsObservation) observe(addresses []netip.Addr, now time.Time) bool {
	if len(addresses) == 0 {
		s.failedProbes = 0
		return false
	}
	for i, addr := range addresses {
		addresses[i] = addr.Unmap()
	}
	slices.SortFunc(addresses, netip.Addr.Compare)
	addresses = slices.Compact(addresses)
	if len(s.baseline) == 0 {
		s.baseline = addresses
		return false
	}
	if slices.Equal(addresses, s.baseline) {
		s.candidate = nil
		s.failedProbes = 0
		return false
	}
	if !slices.Equal(addresses, s.candidate) {
		s.candidate = addresses
		s.changedAt = now
		s.failedProbes = 0
		s.restarted = false
	}
	return !now.Before(s.retryAfter)
}

func (s *ddnsObservation) recordProbe(healthy bool, now time.Time) bool {
	if healthy {
		s.failedProbes = 0
		// The old peer may still respond while the new address propagates.
		if now.Sub(s.changedAt) >= ddnsObservationWindow {
			s.baseline, s.candidate = s.candidate, nil
		}
		return false
	}
	if s.restarted {
		s.retryAfter = now.Add(ddnsRestartCooldown)
		return false
	}
	s.failedProbes++
	return s.failedProbes >= 2
}

// MonitorWireguardDDNS observes address changes for active custom exits. Only a
// changed address set followed by failed traffic probes can trigger a restart.
func (em *ExitManager) MonitorWireguardDDNS(ctx context.Context, interval time.Duration) {
	if interval <= 0 || em.ensureDockerAvailable() != nil {
		return
	}
	observations := make(map[uint]*ddnsObservation)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		if err := em.scanWireguardDDNS(ctx, observations); err != nil && ctx.Err() == nil {
			log.Printf("WireGuard DDNS check failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (em *ExitManager) scanWireguardDDNS(ctx context.Context, observations map[uint]*ddnsObservation) error {
	var exits []models.Exit
	if err := db.DB.WithContext(ctx).Where("enabled AND runtime = ? AND protocol = ? AND provider = ? AND status IN ?",
		"sing-box", "wireguard", "custom", []string{"running", "unhealthy"}).Order("id").Find(&exits).Error; err != nil {
		return err
	}
	active := make(map[uint]bool, len(exits))
	for _, exit := range exits {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		host := ddnsHostname(exit)
		if host == "" {
			continue
		}
		active[exit.ID] = true
		config := sha256.Sum256([]byte(exit.ConfigJSON))
		state := observations[exit.ID]
		if state == nil || state.name != exit.Name || state.config != config {
			state = &ddnsObservation{name: exit.Name, config: config}
			observations[exit.ID] = state
		}
		lookupCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		addresses, err := net.DefaultResolver.LookupNetIP(lookupCtx, "ip", host)
		cancel()
		if err != nil {
			// A transient resolver failure gives us no evidence of a new address.
			state.failedProbes = 0
			continue
		}
		if state.observe(addresses, time.Now()) {
			em.recoverDDNSExit(ctx, exit, state)
		}
	}
	for id := range observations {
		if !active[id] {
			delete(observations, id)
		}
	}
	return nil
}

func (em *ExitManager) recoverDDNSExit(ctx context.Context, exit models.Exit, state *ddnsObservation) {
	unlock, ok := em.tryLockExit(exit.ID)
	if !ok {
		state.failedProbes = 0
		return
	}
	defer unlock()
	// Reload under the same lock as Start, Stop, and Restart.
	var current models.Exit
	if db.DB.WithContext(ctx).First(&current, exit.ID).Error != nil ||
		ddnsHostname(current) == "" || current.Name != exit.Name || current.ConfigJSON != exit.ConfigJSON {
		return
	}
	container := "singbox-" + current.Name
	inspectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	status, err := em.docker.GetContainerStatus(inspectCtx, container)
	cancel()
	if err != nil || status != "running" {
		state.failedProbes = 0
		return
	}
	ip, probeErr := probeSingBoxPublicIPContext(ctx, current.Name)
	if ctx.Err() != nil {
		return
	}
	if !state.recordProbe(probeErr == nil, time.Now()) {
		if probeErr == nil && (current.Status != "running" || current.PublicIP != ip) {
			em.saveDDNSStatus(ctx, current, "running", ip)
		}
		return
	}
	// An edit can save its DB row while waiting to reconcile the container.
	var latest models.Exit
	if db.DB.WithContext(ctx).First(&latest, current.ID).Error != nil ||
		ddnsHostname(latest) == "" || latest.Name != current.Name || latest.ConfigJSON != current.ConfigJSON {
		return
	}
	restartCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	status, err = em.docker.GetContainerStatus(restartCtx, container)
	if err != nil || status != "running" {
		state.failedProbes = 0
		return
	}
	state.failedProbes = 0
	state.restarted = true
	state.retryAfter = time.Now().Add(ddnsRestartCooldown)
	em.logEventContext(restartCtx, current.ID, "ddns_reconnect", "started", "WireGuard DNS address changed and VPN probes failed")
	err = em.docker.RestartContainer(restartCtx, container)
	if err == nil {
		err = em.docker.WaitForContainerRunning(restartCtx, container)
	}
	if err == nil {
		ip, err = probeProviderPublicIPFor(restartCtx, current.Name, 20*time.Second)
	}
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		em.saveDDNSStatus(ctx, current, "unhealthy", "")
		em.logEventContext(ctx, current.ID, "ddns_reconnect", "failed", fmt.Sprintf("WireGuard reconnect failed: %v", err))
		return
	}
	state.baseline, state.candidate = state.candidate, nil
	state.retryAfter = time.Time{}
	em.saveDDNSStatus(ctx, current, "running", ip)
	em.logEventContext(ctx, current.ID, "ddns_reconnect", "success", "WireGuard connected after a DNS change")
}

func (em *ExitManager) saveDDNSStatus(ctx context.Context, exit models.Exit, status, ip string) {
	if err := db.DB.WithContext(ctx).Model(&models.Exit{}).
		Where("id = ? AND name = ? AND enabled AND runtime = ? AND protocol = ? AND config_json = ?",
			exit.ID, exit.Name, exit.Runtime, exit.Protocol, exit.ConfigJSON).
		Updates(map[string]any{"status": status, "public_ip": ip}).Error; err != nil && ctx.Err() == nil {
		log.Printf("Could not save WireGuard DDNS status for %s: %v", exit.Name, err)
	}
}
