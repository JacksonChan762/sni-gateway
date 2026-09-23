package singbox

import "encoding/json"

// BuildGluetunProxyConfig listens on Gluetun's private network namespace.
// Its direct outbound is confined by Gluetun's VPN firewall.
func BuildGluetunProxyConfig() ([]byte, error) {
	return json.Marshal(map[string]any{
		"log":       map[string]any{"level": "info"},
		"inbounds":  []any{map[string]any{"type": "socks", "tag": "ingress", "listen": "0.0.0.0", "listen_port": 1080}},
		"outbounds": []any{map[string]any{"type": "direct", "tag": "vpn"}},
		"dns":       map[string]any{"servers": []any{map[string]any{"type": "udp", "tag": "gluetun-dns", "server": "127.0.0.1"}}, "final": "gluetun-dns", "strategy": "ipv4_only"},
		"route":     map[string]any{"final": "vpn"},
	})
}
