package services

import (
	"testing"
	"time"
)

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
