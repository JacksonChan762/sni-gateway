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

func TestValidateExitRequestRequiresCustomWireguardFields(t *testing.T) {
	err := validateExitRequest(CreateExitRequest{
		Protocol:         "wireguard",
		Provider:         "custom",
		WireguardPrivKey: "private",
	})
	if err == nil {
		t.Fatal("validateExitRequest returned nil, want missing custom field error")
	}
	if !strings.Contains(err.Error(), "wireguard_endpoint_ip") {
		t.Fatalf("error = %q, want missing endpoint IP", err.Error())
	}
}

func TestValidateExitRequestAcceptsCustomWireguardFields(t *testing.T) {
	err := validateExitRequest(CreateExitRequest{
		Protocol:              "wireguard",
		Provider:              "custom",
		WireguardEndpointIP:   "203.0.113.10",
		WireguardEndpointPort: "51820",
		WireguardPublicKey:    "public",
		WireguardPrivKey:      "private",
		WireguardAddresses:    "10.64.222.21/32",
		WireguardDNS:          "1.1.1.1,8.8.8.8:53",
	})
	if err != nil {
		t.Fatalf("validateExitRequest returned error: %v", err)
	}
}

func TestValidateExitRequestRejectsCustomWireguardHostname(t *testing.T) {
	err := validateExitRequest(CreateExitRequest{
		Protocol:              "wireguard",
		Provider:              "custom",
		WireguardEndpointIP:   "vpn.example.com",
		WireguardEndpointPort: "51820",
		WireguardPublicKey:    "public",
		WireguardPrivKey:      "private",
		WireguardAddresses:    "10.64.222.21/32",
	})
	if err == nil || !strings.Contains(err.Error(), "must be an IP address") {
		t.Fatalf("error = %v, want endpoint IP validation error", err)
	}
}

func TestValidateExitRequestRejectsCustomWireguardAddressWithoutCIDR(t *testing.T) {
	err := validateExitRequest(CreateExitRequest{
		Protocol:              "wireguard",
		Provider:              "custom",
		WireguardEndpointIP:   "203.0.113.10",
		WireguardEndpointPort: "51820",
		WireguardPublicKey:    "public",
		WireguardPrivKey:      "private",
		WireguardAddresses:    "10.64.222.21",
	})
	if err == nil || !strings.Contains(err.Error(), "CIDR") {
		t.Fatalf("error = %v, want CIDR validation error", err)
	}
}

func TestValidateExitRequestRejectsCustomWireguardDNSHostname(t *testing.T) {
	err := validateExitRequest(CreateExitRequest{
		Protocol:              "wireguard",
		Provider:              "custom",
		WireguardEndpointIP:   "203.0.113.10",
		WireguardEndpointPort: "51820",
		WireguardPublicKey:    "public",
		WireguardPrivKey:      "private",
		WireguardAddresses:    "10.64.222.21/32",
		WireguardDNS:          "dns.example.com",
	})
	if err == nil || !strings.Contains(err.Error(), "wireguard_dns") {
		t.Fatalf("error = %v, want DNS validation error", err)
	}
}

func TestBuildManualRuleNormalizesPlusPrefix(t *testing.T) {
	rule, err := buildManualRule(CreateRuleRequest{
		Pattern:  " +.example.com ",
		Type:     "auto",
		Priority: 200,
	})
	if err != nil {
		t.Fatalf("buildManualRule returned error: %v", err)
	}

	if rule.Pattern != "example.com" {
		t.Fatalf("pattern = %q, want example.com", rule.Pattern)
	}
	if rule.Type != "suffix" {
		t.Fatalf("type = %q, want suffix", rule.Type)
	}
	if rule.Priority != 200 {
		t.Fatalf("priority = %d, want 200", rule.Priority)
	}
}

func TestBuildManualRuleAutoDetectsPlainPatternAsExact(t *testing.T) {
	rule, err := buildManualRule(CreateRuleRequest{
		Pattern: "example.com",
		Type:    "auto",
	})
	if err != nil {
		t.Fatalf("buildManualRule returned error: %v", err)
	}

	if rule.Pattern != "example.com" {
		t.Fatalf("pattern = %q, want example.com", rule.Pattern)
	}
	if rule.Type != "exact" {
		t.Fatalf("type = %q, want exact", rule.Type)
	}
}

func TestBuildManualRuleDefaultsMissingTypeToAuto(t *testing.T) {
	rule, err := buildManualRule(CreateRuleRequest{
		Pattern: "example.com",
	})
	if err != nil {
		t.Fatalf("buildManualRule returned error: %v", err)
	}

	if rule.Type != "exact" {
		t.Fatalf("type = %q, want exact", rule.Type)
	}
}
