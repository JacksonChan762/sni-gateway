package services

import (
	"net/netip"
	"testing"
	"time"

	"sni-gateway/internal/models"
)

func TestDDNSHostnameOnlyForActiveCustomWireguard(t *testing.T) {
	exit := models.Exit{Enabled: true, Runtime: "sing-box", Protocol: "wireguard", Provider: "custom", Status: "running",
		ConfigJSON: `{"wireguard_endpoint_ip":"vpn.example.net"}`}
	if got := ddnsHostname(exit); got != "vpn.example.net" {
		t.Fatalf("hostname = %q", got)
	}
	for _, tt := range []struct {
		name   string
		change func(*models.Exit)
	}{
		{"disabled", func(e *models.Exit) { e.Enabled = false }},
		{"stopped", func(e *models.Exit) { e.Status = "stopped" }},
		{"provider", func(e *models.Exit) { e.Provider = "mullvad" }},
		{"openvpn", func(e *models.Exit) { e.Protocol = "openvpn" }},
		{"legacy", func(e *models.Exit) { e.Runtime = "gluetun" }},
		{"numeric", func(e *models.Exit) { e.ConfigJSON = `{"wireguard_endpoint_ip":"192.0.2.3"}` }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			edited := exit
			tt.change(&edited)
			if got := ddnsHostname(edited); got != "" {
				t.Fatalf("ineligible exit selected for monitoring: %s", got)
			}
		})
	}
}

func TestDDNSRecoveryRequiresStableChangeAndRepeatedFailures(t *testing.T) {
	now := time.Now()
	ip4 := netip.MustParseAddr("192.0.2.1")
	ip6 := netip.MustParseAddr("2001:db8::1")
	peer := new(ddnsObservation)
	if peer.observe([]netip.Addr{ip4, ip6}, now) {
		t.Fatal("first observation must establish a baseline")
	}
	mapped := netip.MustParseAddr("::ffff:192.0.2.1")
	if peer.observe([]netip.Addr{ip6, mapped, ip4}, now) {
		t.Fatal("reordering, duplicate or mapped addresses must not trigger a probe")
	}
	changed := []netip.Addr{netip.MustParseAddr("2001:db8::2")}
	if !peer.observe(changed, now) || peer.recordProbe(false, now) {
		t.Fatal("first failed probe should not restart the exit")
	}
	if peer.observe(nil, now) || !peer.observe(changed, now) || peer.recordProbe(false, now) {
		t.Fatal("failed DNS lookup interrupts the confirmation sequence")
	}
	if !peer.observe(changed, now) || !peer.recordProbe(false, now) {
		t.Fatal("stable DNS change and consecutive probe failures should restart")
	}
	peer.restarted = true
	peer.retryAfter = now.Add(ddnsRestartCooldown)
	if peer.observe(changed, now.Add(time.Minute)) || !peer.observe(changed, peer.retryAfter) {
		t.Fatal("cooldown should delay probing after an unsuccessful restart")
	}
	if peer.recordProbe(false, peer.retryAfter) {
		t.Fatal("an unsuccessful restart must not loop for the same DNS change")
	}
}

func TestDDNSHealthyChangeKeepsCheckingDuringObservationWindow(t *testing.T) {
	now := time.Now()
	peer := new(ddnsObservation)
	peer.observe([]netip.Addr{netip.MustParseAddr("192.0.2.1")}, now)
	changed := []netip.Addr{netip.MustParseAddr("192.0.2.2")}
	if !peer.observe(changed, now) || peer.recordProbe(true, now) {
		t.Fatal("a healthy exit should not restart")
	}
	if !peer.observe(changed, now.Add(time.Minute)) {
		t.Fatal("a healthy old peer may still be serving during DNS rotation")
	}
	if peer.recordProbe(true, now.Add(ddnsObservationWindow)) || peer.observe(changed, now.Add(ddnsObservationWindow)) {
		t.Fatal("healthy resolution should become the new baseline after the observation window")
	}
}

func TestDDNSRecoveryLockDefersToExitOperations(t *testing.T) {
	em := NewExitManager(nil)
	release := em.lockExit(1)
	if unlock, ok := em.tryLockExit(1); ok {
		unlock()
		t.Error("recovery overlapped an existing operation on the same exit")
	}
	if unlock, ok := em.tryLockExit(2); ok {
		unlock()
	} else {
		t.Error("operations on other exits should not keep this exit from recovering")
	}
	release()
	em.operationMu.Lock()
	defer em.operationMu.Unlock()
	if unlock, ok := em.tryLockExit(1); ok {
		unlock()
		t.Error("recovery overlapped the exclusive Stop All operation")
	}
}
