package docker

import (
	"strings"
	"testing"
)

func TestOrderedGluetunEnvKeepsCustomKeys(t *testing.T) {
	env := orderedGluetunEnv(map[string]string{
		"VPN_SERVICE_PROVIDER": "custom",
		"VPN_TYPE":             "wireguard",
		"WIREGUARD_PRIVATE_KEY": "private",
		"WIREGUARD_ENDPOINT_IP": "203.0.113.10",
		"FIREWALL_INPUT_PORTS":  "10008",
	}, "wireguard")

	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "WIREGUARD_ENDPOINT_IP=203.0.113.10") {
		t.Fatalf("custom env key missing from ordered env: %v", env)
	}
	if !strings.Contains(joined, "VPN_SERVICE_PROVIDER=custom") {
		t.Fatalf("provider missing from ordered env: %v", env)
	}
}
