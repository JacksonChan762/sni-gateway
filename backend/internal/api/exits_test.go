package api

import (
	"strings"
	"testing"

	"sni-gateway/internal/models"
)

func TestBuildExitConfigYAMLPreservesStoredEnvironment(t *testing.T) {
	exit := &models.Exit{
		Name:     "us",
		Runtime:  "gluetun",
		Protocol: "wireguard",
		Provider: "protonvpn",
		Country:  "United States",
		City:     "New York",
		Enabled:  true,
		ConfigJSON: `{
			"services": {
				"gluetun": {
					"environment": [
						"VPN_SERVICE_PROVIDER=old-provider",
						"VPN_TYPE=wireguard",
						"SERVER_COUNTRIES=Old Country",
						"WIREGUARD_PRIVATE_KEY=private",
						"WIREGUARD_ADDRESSES=10.2.0.2/32",
						"FIREWALL_OUTBOUND_SUBNETS=10.0.0.0/8"
					]
				}
			},
			"wireguard_private_key": "private",
			"wireguard_addresses": "10.2.0.2/32"
		}`,
	}

	configYAML, err := buildExitConfigYAML(exit)
	if err != nil {
		t.Fatalf("buildExitConfigYAML returned error: %v", err)
	}

	for _, want := range []string{
		"VPN_SERVICE_PROVIDER=protonvpn",
		"SERVER_COUNTRIES=United States",
		"SERVER_CITIES=New York",
		"WIREGUARD_PRIVATE_KEY=private",
		"WIREGUARD_ADDRESSES=10.2.0.2/32",
		"FIREWALL_OUTBOUND_SUBNETS=10.0.0.0/8",
	} {
		if !strings.Contains(configYAML, want) {
			t.Fatalf("config YAML missing %q:\n%s", want, configYAML)
		}
	}
}
