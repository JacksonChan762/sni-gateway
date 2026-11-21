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
	if len(dnsConfig["servers"].([]any)) != 1 {
		t.Fatal("a literal endpoint should only need VPN DNS")
	}
}

func TestBuildWireguardHostnameWithTuning(t *testing.T) {
	exit := wireguardExit("0.0.0.0/0")
	var raw map[string]string
	if err := json.Unmarshal([]byte(exit.ConfigJSON), &raw); err != nil {
		t.Fatal(err)
	}
	raw["wireguard_endpoint_ip"] = "vpn.example.net."
	raw["wireguard_mtu"] = "1380"
	raw["wireguard_persistent_keepalive_interval"] = "25"
	encoded, _ := json.Marshal(raw)
	exit.ConfigJSON = string(encoded)
	data, err := BuildExitConfig(exit)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	endpoint := config["endpoints"].([]any)[0].(map[string]any)
	peer := endpoint["peers"].([]any)[0].(map[string]any)
	if peer["address"] != "vpn.example.net." || peer["persistent_keepalive_interval"] != float64(25) || endpoint["mtu"] != float64(1380) {
		t.Fatal("peer hostname or profile tuning was lost")
	}
	resolver := endpoint["domain_resolver"].(map[string]any)
	if resolver["server"] != "bootstrap" || resolver["strategy"] != "prefer_ipv4" || resolver["disable_optimistic_cache"] != true {
		t.Fatal("peer should resolve both families without using stale DNS answers")
	}
	dns := config["dns"].(map[string]any)
	servers := dns["servers"].([]any)
	bootstrap := servers[1].(map[string]any)
	if dns["final"] != "vpn-dns" || dns["strategy"] != "ipv4_only" || servers[0].(map[string]any)["detour"] != "vpn" ||
		bootstrap["type"] != "local" || bootstrap["tag"] != "bootstrap" || bootstrap["detour"] != nil {
		t.Fatal("peer-only bootstrap DNS and VPN DNS must remain separate")
	}
	route := config["route"].(map[string]any)
	if route["final"] != "vpn" || route["default_domain_resolver"] != "vpn-dns" {
		t.Fatal("route and destination resolution must use the VPN")
	}
}

func TestBuildWireguardEndpointAndTuningValidation(t *testing.T) {
	for _, tt := range []struct {
		field, value string
		wantError    bool
	}{
		{"wireguard_endpoint_ip", "2001:db8::1", false},
		{"wireguard_endpoint_ip", "vpn.example.net:51820", true},
		{"wireguard_endpoint_ip", "https://vpn.example.net", true},
		{"wireguard_endpoint_ip", "vpn..example.net", true},
		{"wireguard_endpoint_ip", "999.0.0.1", true},
		{"wireguard_mtu", "0", true},
		{"wireguard_mtu", "65536", true},
		{"wireguard_persistent_keepalive_interval", "off", false},
		{"wireguard_persistent_keepalive_interval", "0", false},
		{"wireguard_persistent_keepalive_interval", "25s", true},
		{"wireguard_persistent_keepalive_interval", "65536", true},
	} {
		t.Run(tt.field+"/"+tt.value, func(t *testing.T) {
			exit := wireguardExit("0.0.0.0/0")
			var raw map[string]string
			if err := json.Unmarshal([]byte(exit.ConfigJSON), &raw); err != nil {
				t.Fatal(err)
			}
			raw[tt.field] = tt.value
			encoded, _ := json.Marshal(raw)
			exit.ConfigJSON = string(encoded)
			_, err := BuildExitConfig(exit)
			if tt.wantError && (err == nil || !strings.Contains(err.Error(), tt.field)) || !tt.wantError && err != nil {
				t.Fatalf("unexpected validation result: %v", err)
			}
		})
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
	candidates, err := catalog.Candidates("protonvpn", "wireguard", "United States", "New York")
	if err != nil || len(candidates) < 2 {
		t.Fatalf("need multiple WireGuard endpoints: %v", err)
	}
	exit.EndpointIP = candidates[1].IP
	built, err = BuildExitConfig(exit)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(built, &config); err != nil {
		t.Fatal(err)
	}
	peer = config["endpoints"].([]any)[0].(map[string]any)["peers"].([]any)[0].(map[string]any)
	if peer["address"] != exit.EndpointIP {
		t.Fatalf("selected WireGuard endpoint ignored: %+v", peer)
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
	candidates, err := catalog.Candidates("protonvpn", "openvpn", "Afghanistan", "Kabul", "udp")
	if err != nil || len(candidates) < 2 {
		t.Fatalf("need multiple OpenVPN endpoints: %v", err)
	}
	exit.EndpointIP = candidates[1].IP
	built, err = BuildExitConfig(exit)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(built, &config); err != nil {
		t.Fatal(err)
	}
	remote = config["endpoints"].([]any)[0].(map[string]any)["servers"].([]any)[0].(map[string]any)
	if remote["server"] != exit.EndpointIP {
		t.Fatalf("selected OpenVPN endpoint ignored: %+v", remote)
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
