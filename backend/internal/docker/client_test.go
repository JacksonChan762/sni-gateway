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

func TestIsContainerNamespaceRestartingError(t *testing.T) {
	err := assertErrorString("create relay container: Error response from daemon: cannot join network namespace of container: Container abc is restarting, wait until the container is running")
	if !isContainerNamespaceRestartingError(err) {
		t.Fatal("expected restarting network namespace error to be retryable")
	}
}

func TestContainerStateDetail(t *testing.T) {
	got := containerStateDetail(1, "bad config")
	if got != ": exit code 1, bad config" {
		t.Fatalf("containerStateDetail() = %q", got)
	}
}

type assertErrorString string

func (e assertErrorString) Error() string {
	return string(e)
}
