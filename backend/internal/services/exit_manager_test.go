package services

import (
	"testing"
	"time"

	"sni-gateway/internal/catalog"
	"sni-gateway/internal/models"
)

func TestCountryDoesNotRecreateCitySelectedSingBox(t *testing.T) {
	before := &models.Exit{Runtime: "sing-box", Protocol: "wireguard", Provider: "protonvpn", Country: "Taiwan", City: "Taipei"}
	after := *before
	after.Country = "Hong Kong"
	if exitRuntimeConfigChanged(before, &after) {
		t.Fatal("country filter restarted a city-selected VPN")
	}
	after.City = ""
	if !exitRuntimeConfigChanged(before, &after) {
		t.Fatal("clearing city did not restart VPN")
	}
}

func TestEndpointTrialsKeepsPreviousAndSamplesDistinctIPs(t *testing.T) {
	candidates := []catalog.Server{{IP: "192.0.2.1"}, {IP: "192.0.2.2"}, {IP: "192.0.2.3"}, {IP: "192.0.2.4"}, {IP: "192.0.2.5"}}
	for i := 0; i < 20; i++ {
		trials := endpointTrials(candidates, "192.0.2.3")
		if len(trials) != maxEndpointTrials || trials[0].IP != "192.0.2.3" {
			t.Fatalf("unexpected trials: %+v", trials)
		}
		seen := map[string]bool{}
		for _, server := range trials {
			if seen[server.IP] {
				t.Fatalf("duplicate endpoint: %+v", trials)
			}
			seen[server.IP] = true
		}
	}
}

func TestExitLocksAllowIndependentOperations(t *testing.T) {
	em := NewExitManager(nil)
	unlockFirst := em.lockExit(1)
	otherDone := make(chan struct{})
	go func() {
		unlock := em.lockExit(2)
		unlock()
		close(otherDone)
	}()
	select {
	case <-otherDone:
	case <-time.After(time.Second):
		t.Fatal("independent exit waited for first exit")
	}

	sameDone := make(chan struct{})
	go func() {
		unlock := em.lockExit(1)
		unlock()
		close(sameDone)
	}()
	select {
	case <-sameDone:
		t.Fatal("same exit operation overlapped")
	case <-time.After(20 * time.Millisecond):
	}
	unlockFirst()
	select {
	case <-sameDone:
	case <-time.After(time.Second):
		t.Fatal("same exit operation remained blocked")
	}
}

func TestChangedRuntimeNamesIncludesOldNameOnRename(t *testing.T) {
	got := changedRuntimeNames("old", "new")
	if len(got) != 2 || got[0] != "old" || got[1] != "new" {
		t.Fatalf("changedRuntimeNames() = %#v, want old and new", got)
	}
}

func TestExtractStoredGluetunEnvPreservesCustomKeys(t *testing.T) {
	raw := map[string]interface{}{
		"services": map[string]interface{}{
			"gluetun": map[string]interface{}{
				"environment": []interface{}{
					"VPN_SERVICE_PROVIDER=custom",
					"VPN_TYPE=wireguard",
					"WIREGUARD_ENDPOINT_IP=203.0.113.10",
				},
			},
		},
	}

	env := extractStoredGluetunEnv(raw)
	if env["WIREGUARD_ENDPOINT_IP"] != "203.0.113.10" {
		t.Fatalf("custom env value = %q, want 203.0.113.10", env["WIREGUARD_ENDPOINT_IP"])
	}
	if env["VPN_SERVICE_PROVIDER"] != "custom" {
		t.Fatalf("provider = %q, want custom", env["VPN_SERVICE_PROVIDER"])
	}
}
