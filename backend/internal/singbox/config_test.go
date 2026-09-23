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
