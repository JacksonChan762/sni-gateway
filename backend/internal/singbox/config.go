package singbox

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
	"strings"

	"sni-gateway/internal/catalog"
	"sni-gateway/internal/models"
	"sni-gateway/internal/provider"
)

var wireguardHostnameLabel = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

// WireGuard accepts an IP literal or a hostname, with the port supplied separately.
func validWireguardHostname(host string) bool {
	host = strings.TrimSuffix(host, ".")
	if len(host) == 0 || len(host) > 253 || strings.Trim(host, "0123456789.") == "" {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if !wireguardHostnameLabel.MatchString(label) {
			return false
		}
	}
	return true
}

// BuildExitConfig creates an isolated SOCKS ingress whose only route is the VPN.
func BuildExitConfig(exit models.Exit) ([]byte, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(exit.ConfigJSON), &raw); err != nil {
		return nil, fmt.Errorf("invalid exit configuration: %w", err)
	}
	if exit.Protocol == "openvpn" {
		return buildOpenVPNConfig(exit, raw)
	}
	if exit.Protocol != "wireguard" {
		return nil, errors.New("protocol must be wireguard or openvpn")
	}
	field := func(name string) string { value, _ := raw[name].(string); return strings.TrimSpace(value) }
	privateKey, publicKey := field("wireguard_private_key"), field("wireguard_public_key")
	endpoint, portText := field("wireguard_endpoint_ip"), field("wireguard_endpoint_port")
	if autoServerEnabled(raw) {
		if !provider.WireGuard(exit.Provider) {
			return nil, fmt.Errorf("%s has no supported WireGuard provider configuration", exit.Provider)
		}
		var server catalog.Server
		var err error
		if exit.EndpointIP != "" {
			server, err = catalog.SelectIP(exit.Provider, "wireguard", exit.Country, exit.City, exit.EndpointIP)
		} else {
			server, err = catalog.Select(exit.Provider, "wireguard", exit.Country, exit.City, exit.Name)
		}
		if err != nil {
			return nil, err
		}
		endpoint, publicKey = server.IP, server.PublicKey
		if portText == "" {
			port, _ := provider.WireGuardPort(exit.Provider)
			portText = strconv.Itoa(int(port))
		}
	}
	_, ipErr := netip.ParseAddr(endpoint)
	hasHostname := ipErr != nil
	if hasHostname && !validWireguardHostname(endpoint) {
		return nil, errors.New("wireguard_endpoint_ip must be an IP address or DNS hostname without a port")
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
	addressText := field("wireguard_addresses")
	if addressText == "" && autoServerEnabled(raw) {
		addressText = provider.DefaultAddresses(exit.Provider)
	}
	addresses, err := prefixes(addressText)
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
	if value := field("wireguard_persistent_keepalive_interval"); value != "" && !strings.EqualFold(value, "off") {
		interval, err := strconv.ParseUint(value, 10, 16)
		if err != nil {
			return nil, errors.New("wireguard_persistent_keepalive_interval must be 0–65535 seconds or off")
		}
		peer["persistent_keepalive_interval"] = interval
	}
	wireguard := map[string]any{"type": "wireguard", "tag": "vpn", "address": addresses, "private_key": privateKey, "peers": []any{peer}}
	if value := field("wireguard_mtu"); value != "" {
		mtu, err := strconv.ParseUint(value, 10, 16)
		minMTU := uint64(68)
		if hasIPv6 {
			minMTU = 1280
		}
		if err != nil || mtu < minMTU {
			return nil, fmt.Errorf("wireguard_mtu must be %d–65535 bytes", minMTU)
		}
		wireguard["mtu"] = mtu
	}
	servers := []any{dns}
	if hasHostname {
		// Resolve the peer before the tunnel is usable. Leave destination DNS in the VPN.
		servers = append(servers, map[string]any{"type": "local", "tag": "bootstrap"})
		wireguard["domain_resolver"] = map[string]any{
			"server": "bootstrap", "strategy": "prefer_ipv4", "disable_optimistic_cache": true,
		}
	}
	dnsConfig := map[string]any{"servers": servers, "final": "vpn-dns"}
	if !hasIPv6 {
		dnsConfig["strategy"] = "ipv4_only"
	}
	config := map[string]any{
		"log":       map[string]any{"level": "info"},
		"inbounds":  []any{map[string]any{"type": "socks", "tag": "ingress", "listen": "0.0.0.0", "listen_port": 1080}},
		"endpoints": []any{wireguard},
		"dns":       dnsConfig,
		"route":     map[string]any{"final": "vpn", "default_domain_resolver": "vpn-dns"},
	}
	return json.Marshal(config)
}

func buildOpenVPNConfig(exit models.Exit, raw map[string]any) ([]byte, error) {
	field := func(name string) string { value, _ := raw[name].(string); return strings.TrimSpace(value) }
	profile := field("openvpn_profile")
	providerResolved := false
	if profile == "" && autoServerEnabled(raw) {
		var err error
		profile, err = buildProviderOpenVPNProfile(exit, field("openvpn_cert"), field("openvpn_key"), field("openvpn_key_passphrase"))
		if err != nil {
			return nil, err
		}
		providerResolved = true
	}
	endpoint, hasHostname, err := parseOpenVPNProfile(profile, field("openvpn_user"), field("openvpn_password"), providerResolved && exit.Provider == "ivpn")
	if err != nil {
		return nil, err
	}
	if autoServerEnabled(raw) && !providerResolved {
		remotes := endpoint["servers"].([]map[string]any)
		remote := remotes[0]
		network, _ := remote["network"].(string)
		if network == "" {
			network, _ = endpoint["network"].(string)
		}
		var server catalog.Server
		if exit.EndpointIP != "" {
			server, err = catalog.SelectIP(exit.Provider, "openvpn", exit.Country, exit.City, exit.EndpointIP, network)
		} else {
			server, err = catalog.Select(exit.Provider, "openvpn", exit.Country, exit.City, exit.Name, network)
		}
		if err != nil {
			return nil, err
		}
		remote["server"] = server.IP
		endpoint["servers"] = []map[string]any{remote}
		hasHostname = false
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

func autoServerEnabled(raw map[string]any) bool {
	switch value := raw["auto_server"].(type) {
	case bool:
		return value
	case string:
		return strings.EqualFold(strings.TrimSpace(value), "true")
	}
	return false
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
