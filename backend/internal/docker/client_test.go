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

func TestBuildGluetunEnvMapOmitsBlankExtraEnv(t *testing.T) {
	env := buildGluetunEnvMap(GluetunConfig{
		Name:         "custom",
		Protocol:     "wireguard",
		Provider:     "custom",
		RelayPort:    10008,
		WireguardKey: "private",
		WireguardAddr: "10.64.222.21/32",
		ExtraEnv: map[string]string{
			"WIREGUARD_ENDPOINT_IP":                  "203.0.113.10",
			"WIREGUARD_ENDPOINT_PORT":                "51820",
			"WIREGUARD_PUBLIC_KEY":                   "public",
			"WIREGUARD_PRESHARED_KEY":                "",
			"WIREGUARD_MTU":                          "",
			"WIREGUARD_PERSISTENT_KEEPALIVE_INTERVAL": " ",
		},
	})

	if env["WIREGUARD_ENDPOINT_IP"] != "203.0.113.10" {
		t.Fatalf("endpoint IP = %q, want 203.0.113.10", env["WIREGUARD_ENDPOINT_IP"])
	}
	if _, ok := env["WIREGUARD_MTU"]; ok {
		t.Fatalf("blank WIREGUARD_MTU should be omitted: %v", env)
	}
	if _, ok := env["WIREGUARD_PERSISTENT_KEEPALIVE_INTERVAL"]; ok {
		t.Fatalf("blank keepalive should be omitted: %v", env)
	}
	if _, ok := env["WIREGUARD_PRESHARED_KEY"]; ok {
		t.Fatalf("blank preshared key should be omitted: %v", env)
	}
}

func TestBuildGluetunEnvMapDoesNotOverwriteStoredCredentialsWithBlankFields(t *testing.T) {
	env := buildGluetunEnvMap(GluetunConfig{
		Name:     "custom",
		Protocol: "wireguard",
		Provider: "custom",
		ExtraEnv: map[string]string{
			"WIREGUARD_PRIVATE_KEY": "stored-private",
			"WIREGUARD_ADDRESSES":   "10.64.222.21/32",
		},
	})

	if env["WIREGUARD_PRIVATE_KEY"] != "stored-private" {
		t.Fatalf("private key = %q, want stored-private", env["WIREGUARD_PRIVATE_KEY"])
	}
	if env["WIREGUARD_ADDRESSES"] != "10.64.222.21/32" {
		t.Fatalf("addresses = %q, want stored address", env["WIREGUARD_ADDRESSES"])
	}
}

func TestBuildGluetunEnvMapConvertsWireguardDNSToGluetunDNS(t *testing.T) {
	env := buildGluetunEnvMap(GluetunConfig{
		Name:     "custom",
		Protocol: "wireguard",
		Provider: "custom",
		ExtraEnv: map[string]string{
			"WIREGUARD_DNS": "1.1.1.1, 8.8.8.8:53",
		},
	})

	if _, ok := env["WIREGUARD_DNS"]; ok {
		t.Fatalf("WIREGUARD_DNS should not be passed directly to gluetun: %v", env)
	}
	if env["DNS_UPSTREAM_RESOLVER_TYPE"] != "plain" {
		t.Fatalf("resolver type = %q, want plain", env["DNS_UPSTREAM_RESOLVER_TYPE"])
	}
	if env["DNS_UPSTREAM_PLAIN_ADDRESSES"] != "1.1.1.1:53,8.8.8.8:53" {
		t.Fatalf("plain DNS = %q", env["DNS_UPSTREAM_PLAIN_ADDRESSES"])
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
