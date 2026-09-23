package singbox

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"sni-gateway/internal/catalog"
	"sni-gateway/internal/models"
)

func wireguardExit(allowed string) models.Exit {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	fields := map[string]string{
		"wireguard_private_key":   key,
		"wireguard_public_key":    key,
		"wireguard_endpoint_ip":   "203.0.113.10",
		"wireguard_endpoint_port": "51820",
		"wireguard_addresses":     "10.2.0.2/32",
		"wireguard_dns":           "9.9.9.9:5353",
		"wireguard_allowed_ips":   allowed,
	}
	data, _ := json.Marshal(fields)
	return models.Exit{Protocol: "wireguard", ConfigJSON: string(data)}
}

func TestBuildExitConfigRoutesAndDNSOverVPN(t *testing.T) {
	data, err := BuildExitConfig(wireguardExit("0.0.0.0/0, ::/0"))
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	route := config["route"].(map[string]any)
	if route["final"] != "vpn" || route["default_domain_resolver"] != "vpn-dns" {
		t.Fatalf("VPN-only route missing: %s", data)
	}
	dnsConfig := config["dns"].(map[string]any)
	if dnsConfig["strategy"] != "ipv4_only" {
		t.Fatalf("IPv4-only tunnel should resolve IPv4 destinations: %s", data)
	}
	dns := dnsConfig["servers"].([]any)[0].(map[string]any)
	if dns["detour"] != "vpn" || dns["server"] != "9.9.9.9" || dns["server_port"] != float64(5353) {
		t.Fatalf("VPN DNS missing: %s", data)
	}
	if strings.Contains(string(data), `"type":"direct"`) {
		t.Fatal("unexpected direct outbound")
	}
}

func TestBuildExitConfigRejectsNonVPNOrIncompletePeer(t *testing.T) {
	if _, err := BuildExitConfig(wireguardExit("10.0.0.0/8")); err == nil {
		t.Fatal("restricted peer must not become a direct fallback")
	}
	exit := wireguardExit("0.0.0.0/0")
	exit.Protocol = "openvpn"
	if _, err := BuildExitConfig(exit); err == nil {
		t.Fatal("unsupported protocol accepted")
	}
}

func TestAutomaticServerChangesWireguardEndpoint(t *testing.T) {
	exit := wireguardExit("0.0.0.0/0")
	exit.Name, exit.Provider, exit.Country, exit.City = "auto-us", "protonvpn", "United States", "New York"
	var raw map[string]any
	if err := json.Unmarshal([]byte(exit.ConfigJSON), &raw); err != nil {
		t.Fatal(err)
	}
	raw["auto_server"] = true
	delete(raw, "wireguard_endpoint_ip")
	delete(raw, "wireguard_public_key")
	encoded, _ := json.Marshal(raw)
	exit.ConfigJSON = string(encoded)
	built, err := BuildExitConfig(exit)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(built, &config); err != nil {
		t.Fatal(err)
	}
	peer := config["endpoints"].([]any)[0].(map[string]any)["peers"].([]any)[0].(map[string]any)
	if peer["address"] == "203.0.113.10" || peer["public_key"] == "" {
		t.Fatalf("catalog server not applied: %+v", peer)
	}
}

func TestProviderWireGuardNeedsOnlyProviderCredentials(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	for _, test := range []struct {
		provider, country, city, address string
		port                             float64
	}{
		{"protonvpn", "Taiwan", "Taipei", "10.2.0.2/32", 51820},
		{"nordvpn", "United States", "New York", "10.5.0.2/32", 51820},
	} {
		raw, _ := json.Marshal(map[string]any{"auto_server": true, "wireguard_private_key": key})
		exit := models.Exit{Name: "provider-test", Protocol: "wireguard", Provider: test.provider,
			Country: test.country, City: test.city, ConfigJSON: string(raw)}
		built, err := BuildExitConfig(exit)
		if err != nil {
			t.Fatal(err)
		}
		var config map[string]any
		if err := json.Unmarshal(built, &config); err != nil {
			t.Fatal(err)
		}
		endpoint := config["endpoints"].([]any)[0].(map[string]any)
		peer := endpoint["peers"].([]any)[0].(map[string]any)
		if peer["port"] != test.port || endpoint["address"].([]any)[0] != test.address || peer["public_key"] == "" {
			t.Fatalf("incomplete %s provider configuration: %s", test.provider, built)
		}
	}
}

func TestAutomaticServerChangesOpenVPNRemote(t *testing.T) {
	fields := map[string]any{"auto_server": true, "openvpn_profile": "client\nproto udp\nremote 203.0.113.10 1194\n<ca>\nTEST CA\n</ca>\n"}
	encoded, _ := json.Marshal(fields)
	exit := models.Exit{Name: "auto-af", Protocol: "openvpn", Provider: "protonvpn", Country: "Afghanistan", City: "Kabul", ConfigJSON: string(encoded)}
	built, err := BuildExitConfig(exit)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(built, &config); err != nil {
		t.Fatal(err)
	}
	remote := config["endpoints"].([]any)[0].(map[string]any)["servers"].([]any)[0].(map[string]any)
	if remote["server"] == "203.0.113.10" || remote["server_port"] != float64(1194) {
		t.Fatalf("catalog remote not applied: %+v", remote)
	}
}

func TestProviderOpenVPNTemplatesBuildForCatalog(t *testing.T) {
	providers, _, err := catalog.ListProviders()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range providers {
		if item.OpenVPN == 0 || item.Name == "ovpn" {
			continue
		}
		country := ""
		for _, candidate := range item.Countries {
			if _, err := catalog.Select(item.Name, "openvpn", candidate, "", "template-test"); err == nil {
				country = candidate
				break
			}
		}
		if country == "" {
			t.Errorf("%s has no selectable OpenVPN country", item.Name)
			continue
		}
		fields, _ := json.Marshal(map[string]any{"auto_server": true, "openvpn_user": "user", "openvpn_password": "password"})
		exit := models.Exit{Name: "template-test", Protocol: "openvpn", Provider: item.Name, Country: country, ConfigJSON: string(fields)}
		if _, err := BuildExitConfig(exit); err != nil {
			t.Errorf("%s: %v", item.Name, err)
		}
	}
}
