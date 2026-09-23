package singbox

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

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
