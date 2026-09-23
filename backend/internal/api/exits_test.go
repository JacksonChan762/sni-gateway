package api

import (
	"encoding/base64"
	"strings"
	"testing"

	"sni-gateway/internal/models"
	"sni-gateway/internal/singbox"
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
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	err := validateExitRequest(CreateExitRequest{
		Protocol:              "wireguard",
		Provider:              "custom",
		WireguardEndpointIP:   "203.0.113.10",
		WireguardEndpointPort: "51820",
		WireguardPublicKey:    key,
		WireguardPrivKey:      key,
		WireguardAddresses:    "10.64.222.21/32",
		WireguardDNS:          "1.1.1.1,8.8.8.8:53",
	})
	if err != nil {
		t.Fatalf("validateExitRequest returned error: %v", err)
	}
}

func TestValidateExitRequestRejectsIncompleteVPNConfig(t *testing.T) {
	if err := validateExitRequest(CreateExitRequest{Protocol: "wireguard", Provider: "protonvpn", WireguardPrivKey: "key"}); err == nil || !strings.Contains(err.Error(), "wireguard_endpoint_ip") {
		t.Fatalf("provider-only exit accepted: %v", err)
	}
	if err := validateExitRequest(CreateExitRequest{Protocol: "openvpn", OpenVPNUser: "user", OpenVPNPassword: "pass"}); err == nil || !strings.Contains(err.Error(), "OpenVPN profile") {
		t.Fatalf("OpenVPN credentials accepted: %v", err)
	}
}

func TestValidateSingBoxProviderCredentialsWithoutProfiles(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	auto := true
	for _, request := range []CreateExitRequest{
		{Name: "proton-wg", Runtime: "sing-box", Protocol: "wireguard", Provider: "protonvpn", Country: "Taiwan", City: "Taipei", AutoServer: &auto, WireguardPrivKey: key},
		{Name: "proton-ovpn", Runtime: "sing-box", Protocol: "openvpn", Provider: "protonvpn", Country: "Taiwan", City: "Taipei", AutoServer: &auto, OpenVPNUser: "test", OpenVPNPassword: "test"},
	} {
		if err := validateExitRequest(request); err != nil {
			t.Fatalf("%s: %v", request.Name, err)
		}
	}
}

func TestOpenVPNExitConfigRoundTrip(t *testing.T) {
	profile := "client\nremote 203.0.113.10 1194\n<ca>\nTEST CA\n</ca>\n"
	req := CreateExitRequest{Protocol: "openvpn", OpenVPNProfile: profile, OpenVPNUser: "alice", OpenVPNPassword: "secret"}
	if err := validateExitRequest(req); err != nil {
		t.Fatal(err)
	}
	raw, err := buildExitConfigJSON(req)
	if err != nil {
		t.Fatal(err)
	}
	exit := models.Exit{Protocol: "openvpn", ConfigJSON: raw}
	if err := normalizeExitConfig(&exit); err != nil {
		t.Fatal(err)
	}
	if _, err := singbox.BuildExitConfig(exit); err != nil {
		t.Fatal(err)
	}
	if parsed := parseExitCredentials(exit.ConfigJSON); parsed["openvpn_profile"] != strings.TrimSpace(profile) || parsed["openvpn_password"] != "secret" {
		t.Fatal("OpenVPN profile or password lost during normalization")
	}
	yamlText, err := buildExitConfigYAML(&exit)
	if err != nil || !strings.Contains(yamlText, "openvpn_profile:") || !strings.Contains(yamlText, "remote 203.0.113.10") {
		t.Fatalf("OpenVPN YAML export failed: %v", err)
	}
}

func TestNormalizeLegacyWireguardEnvironmentForSingBox(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	exit := models.Exit{Protocol: "wireguard", ConfigJSON: `{"services":{"gluetun":{"environment":["WIREGUARD_PRIVATE_KEY=` + key + `","WIREGUARD_PUBLIC_KEY=` + key + `","WIREGUARD_ENDPOINT_IP=203.0.113.10","WIREGUARD_ENDPOINT_PORT=51820","WIREGUARD_ADDRESSES=10.2.0.2/32"]}}}`}
	if err := normalizeExitConfig(&exit); err != nil {
		t.Fatal(err)
	}
	if _, err := singbox.BuildExitConfig(exit); err != nil {
		t.Fatalf("normalized config is unusable: %v", err)
	}
	if strings.Contains(exit.ConfigJSON, "services") {
		t.Fatal("legacy Compose block retained")
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

func TestAutomaticServerConfigRoundTrip(t *testing.T) {
	auto := true
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	req := CreateExitRequest{Name: "auto-us", Protocol: "wireguard", Provider: "protonvpn", Country: "United States", City: "New York", AutoServer: &auto,
		WireguardPrivKey: key, WireguardAddresses: "10.2.0.2/32", WireguardEndpointPort: "51820"}
	if err := validateExitRequest(req); err != nil {
		t.Fatal(err)
	}
	encoded, err := buildExitConfigJSON(req)
	if err != nil {
		t.Fatal(err)
	}
	exit := &models.Exit{Name: req.Name, Protocol: req.Protocol, Provider: req.Provider, Country: req.Country, City: req.City, ConfigJSON: encoded}
	if err := normalizeExitConfig(exit); err != nil {
		t.Fatal(err)
	}
	if err := validateExitConfig(exit); err != nil {
		t.Fatal(err)
	}
	yamlText, err := buildExitConfigYAML(exit)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(yamlText, "AUTO_SERVER=true") || !strings.Contains(yamlText, "SERVER_CITIES=New York") {
		t.Fatalf("lost automatic selection: %s", yamlText)
	}
}
