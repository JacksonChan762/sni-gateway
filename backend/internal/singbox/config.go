package singbox

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"sni-gateway/internal/models"
)

// BuildExitConfig creates an isolated SOCKS ingress whose only route is the VPN.
// Legacy provider/region credentials cannot identify a WireGuard peer.
func BuildExitConfig(exit models.Exit) ([]byte, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(exit.ConfigJSON), &raw); err != nil {
		return nil, fmt.Errorf("invalid exit configuration: %w", err)
	}
	if exit.Protocol == "openvpn" {
		return buildOpenVPNConfig(raw)
	}
	if exit.Protocol != "wireguard" {
		return nil, errors.New("protocol must be wireguard or openvpn")
	}
	field := func(name string) string { value, _ := raw[name].(string); return strings.TrimSpace(value) }
	privateKey, publicKey := field("wireguard_private_key"), field("wireguard_public_key")
	endpoint, portText := field("wireguard_endpoint_ip"), field("wireguard_endpoint_port")
	if _, err := netip.ParseAddr(endpoint); err != nil {
		return nil, errors.New("WireGuard endpoint IP is required")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return nil, errors.New("WireGuard endpoint port must be 1–65535")
	}
	for label, key := range map[string]string{"private": privateKey, "peer public": publicKey} {
		decoded, err := base64.StdEncoding.DecodeString(key)
		if err != nil || len(decoded) != 32 {
			return nil, fmt.Errorf("WireGuard %s key must be a 32-byte base64 key", label)
		}
	}
	addresses, err := prefixes(field("wireguard_addresses"))
	if err != nil || len(addresses) == 0 {
		return nil, errors.New("WireGuard interface addresses must be CIDR prefixes")
	}
	hasIPv6 := false
	for _, value := range addresses {
		prefix, _ := netip.ParsePrefix(value)
		hasIPv6 = hasIPv6 || prefix.Addr().Is6()
	}
	allowed := []string{"0.0.0.0/0", "::/0"}
	if value := field("wireguard_allowed_ips"); value != "" {
		allowed, err = prefixes(value)
		if err != nil {
			return nil, errors.New("WireGuard allowed IPs must be CIDR prefixes")
		}
	}
	if !contains(allowed, "0.0.0.0/0") {
		return nil, errors.New("WireGuard peer must allow 0.0.0.0/0 to avoid a direct fallback")
	}
	dnsServer := "1.1.1.1"
	dnsPort := uint16(53)
	if value := field("wireguard_dns"); value != "" {
		dnsServer = strings.TrimSpace(strings.Split(value, ",")[0])
		if addressPort, err := netip.ParseAddrPort(dnsServer); err == nil {
			dnsServer, dnsPort = addressPort.Addr().String(), addressPort.Port()
			if dnsPort == 0 {
				return nil, errors.New("WireGuard DNS port must be greater than zero")
			}
		} else if _, err := netip.ParseAddr(dnsServer); err != nil {
			return nil, errors.New("WireGuard DNS must be an IP address with an optional port")
		}
	}
	dns := map[string]any{"type": "udp", "tag": "vpn-dns", "server": dnsServer, "server_port": dnsPort, "detour": "vpn"}
	peer := map[string]any{"address": endpoint, "port": port, "public_key": publicKey, "allowed_ips": allowed}
	if key := field("wireguard_preshared_key"); key != "" {
		decoded, err := base64.StdEncoding.DecodeString(key)
		if err != nil || len(decoded) != 32 {
			return nil, errors.New("WireGuard preshared key must be a 32-byte base64 key")
		}
		peer["pre_shared_key"] = key
	}
	dnsConfig := map[string]any{"servers": []any{dns}, "final": "vpn-dns"}
	if !hasIPv6 {
		dnsConfig["strategy"] = "ipv4_only"
	}
	config := map[string]any{
		"log":       map[string]any{"level": "info"},
		"inbounds":  []any{map[string]any{"type": "socks", "tag": "ingress", "listen": "0.0.0.0", "listen_port": 1080}},
		"endpoints": []any{map[string]any{"type": "wireguard", "tag": "vpn", "address": addresses, "private_key": privateKey, "peers": []any{peer}}},
		"dns":       dnsConfig,
		"route":     map[string]any{"final": "vpn", "default_domain_resolver": "vpn-dns"},
	}
	return json.Marshal(config)
}

func buildOpenVPNConfig(raw map[string]any) ([]byte, error) {
	field := func(name string) string { value, _ := raw[name].(string); return strings.TrimSpace(value) }
	endpoint, hasHostname, err := parseOpenVPNProfile(field("openvpn_profile"), field("openvpn_user"), field("openvpn_password"))
	if err != nil {
		return nil, err
	}
	dns := map[string]any{"type": "udp", "tag": "vpn-dns", "server": "1.1.1.1", "detour": "vpn"}
	servers := []any{dns}
	if hasHostname {
		servers = append(servers, map[string]any{"type": "local", "tag": "bootstrap"})
		endpoint["domain_resolver"] = "bootstrap"
	}
	config := map[string]any{
		"log":       map[string]any{"level": "info"},
		"inbounds":  []any{map[string]any{"type": "socks", "tag": "ingress", "listen": "0.0.0.0", "listen_port": 1080}},
		"endpoints": []any{endpoint},
		"dns":       map[string]any{"servers": servers, "final": "vpn-dns"},
		"route":     map[string]any{"final": "vpn", "default_domain_resolver": "vpn-dns"},
	}
	return json.Marshal(config)
}

func prefixes(value string) ([]string, error) {
	var result []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, err := netip.ParsePrefix(part); err != nil {
			return nil, err
		}
		result = append(result, part)
	}
	return result, nil
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
