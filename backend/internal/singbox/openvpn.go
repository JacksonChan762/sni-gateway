package singbox

import (
	"bufio"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

var openVPNHost = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)

// parseOpenVPNProfile accepts portable inline profiles. Unsupported directives
// fail explicitly so a VPN profile is never silently weakened on import.
func parseOpenVPNProfile(profile, username, password string) (map[string]any, bool, error) {
	if len(profile) == 0 || len(profile) > 512*1024 {
		return nil, false, errors.New("OpenVPN profile must be 1–524288 bytes")
	}
	endpoint := map[string]any{"type": "openvpn-client", "tag": "vpn", "mode": "tls"}
	tls := map[string]any{}
	var remotes []map[string]any
	blocks := make(map[string]string)
	network := "udp"
	usesHostname := false
	requiresCredentials := false
	section := ""
	var content strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(profile))
	scanner.Buffer(make([]byte, 4096), 512*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if section != "" {
			if line == "</"+section+">" {
				blocks[section] = content.String()
				section = ""
				content.Reset()
				continue
			}
			content.WriteString(scanner.Text() + "\n")
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "<") && strings.HasSuffix(line, ">") {
			section = strings.TrimSuffix(strings.TrimPrefix(line, "<"), ">")
			switch section {
			case "ca", "cert", "key", "tls-auth", "tls-crypt", "tls-crypt-v2":
				continue
			}
			return nil, false, fmt.Errorf("unsupported OpenVPN inline section <%s>", section)
		}
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}
		key, args := strings.ToLower(parts[0]), parts[1:]
		argument := func() (string, error) {
			if len(args) == 0 {
				return "", fmt.Errorf("line %d: %s requires a value", lineNumber, key)
			}
			return args[0], nil
		}
		switch key {
		case "remote":
			if len(args) < 1 || len(args) > 3 {
				return nil, false, fmt.Errorf("line %d: remote requires host, optional port and protocol", lineNumber)
			}
			host := args[0]
			if _, err := netip.ParseAddr(host); err != nil {
				if !openVPNHost.MatchString(host) || !strings.Contains(host, ".") {
					return nil, false, fmt.Errorf("line %d: invalid OpenVPN remote host", lineNumber)
				}
				usesHostname = true
			}
			port := 1194
			if len(args) >= 2 {
				var err error
				port, err = strconv.Atoi(args[1])
				if err != nil || port < 1 || port > 65535 {
					return nil, false, fmt.Errorf("line %d: invalid OpenVPN remote port", lineNumber)
				}
			}
			server := map[string]any{"server": host, "server_port": port}
			if len(args) == 3 {
				proto, err := openVPNProtocol(args[2])
				if err != nil {
					return nil, false, fmt.Errorf("line %d: %w", lineNumber, err)
				}
				server["network"] = proto
			}
			remotes = append(remotes, server)
		case "proto":
			value, err := argument()
			if err != nil {
				return nil, false, err
			}
			network, err = openVPNProtocol(value)
			if err != nil {
				return nil, false, fmt.Errorf("line %d: %w", lineNumber, err)
			}
		case "auth-user-pass":
			if len(args) > 0 {
				return nil, false, fmt.Errorf("line %d: external auth-user-pass files are unsupported; enter credentials separately", lineNumber)
			}
			requiresCredentials = true
		case "dev":
			value, err := argument()
			if err != nil {
				return nil, false, err
			}
			if value != "tun" && !strings.HasPrefix(value, "tun") {
				return nil, false, fmt.Errorf("line %d: only tun devices are supported", lineNumber)
			}
		case "dev-type":
			value, err := argument()
			if err != nil {
				return nil, false, err
			}
			if value != "tun" {
				return nil, false, fmt.Errorf("line %d: only tun devices are supported", lineNumber)
			}
		case "ca", "cert", "key", "tls-auth", "tls-crypt", "tls-crypt-v2":
			return nil, false, fmt.Errorf("line %d: external %s files are unsupported; embed the data in <%s> tags", lineNumber, key, key)
		case "remote-cert-tls":
			value, err := argument()
			if err != nil {
				return nil, false, err
			}
			if value != "server" {
				return nil, false, fmt.Errorf("line %d: only remote-cert-tls server is supported", lineNumber)
			}
			tls["remote_certificate_tls"] = "server"
		case "verify-x509-name":
			value, err := argument()
			if err != nil {
				return nil, false, err
			}
			tls["server_name"] = strings.Trim(value, `"'`)
			if len(args) > 1 {
				kind := args[1]
				if kind != "name" && kind != "name-prefix" && kind != "subject" {
					return nil, false, fmt.Errorf("line %d: unsupported verify-x509-name type", lineNumber)
				}
				tls["server_name_type"] = kind
			}
		case "key-direction":
			value, err := argument()
			if err != nil {
				return nil, false, err
			}
			if value != "0" && value != "1" {
				return nil, false, fmt.Errorf("line %d: invalid key-direction", lineNumber)
			}
			if value == "1" {
				endpoint["key_direction"] = "client"
			} else {
				endpoint["key_direction"] = "server"
			}
		case "cipher":
			value, err := argument()
			if err != nil {
				return nil, false, err
			}
			endpoint["data_ciphers_fallback"] = value
		case "data-ciphers", "ncp-ciphers":
			value, err := argument()
			if err != nil {
				return nil, false, err
			}
			endpoint["data_ciphers"] = strings.Split(value, ":")
		case "auth":
			value, err := argument()
			if err != nil {
				return nil, false, err
			}
			endpoint["auth"] = value
		case "tun-mtu":
			value, err := argument()
			if err != nil {
				return nil, false, err
			}
			mtu, err := strconv.Atoi(value)
			if err != nil || mtu < 576 || mtu > 9000 {
				return nil, false, fmt.Errorf("line %d: invalid tun-mtu", lineNumber)
			}
			endpoint["mtu"] = mtu
		case "remote-random":
			endpoint["remote_random"] = true
		case "client", "nobind", "persist-key", "persist-tun", "resolv-retry", "verb", "mute", "auth-nocache", "pull", "tls-client", "setenv", "setenv-safe", "block-outside-dns", "ignore-unknown-option", "explicit-exit-notify", "connect-retry", "connect-timeout", "float":
			// Process-local options or behavior already provided by the endpoint.
		default:
			return nil, false, fmt.Errorf("line %d: unsupported OpenVPN option %q", lineNumber, key)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, false, err
	}
	if section != "" {
		return nil, false, fmt.Errorf("unterminated OpenVPN <%s> section", section)
	}
	if len(remotes) == 0 {
		return nil, false, errors.New("OpenVPN profile needs at least one remote server")
	}
	if strings.TrimSpace(blocks["ca"]) == "" {
		return nil, false, errors.New("OpenVPN profile needs an inline <ca> certificate")
	}
	if (blocks["cert"] == "") != (blocks["key"] == "") {
		return nil, false, errors.New("OpenVPN client certificate and key must both be present")
	}
	if username != "" || password != "" {
		if username == "" || password == "" {
			return nil, false, errors.New("OpenVPN username and password must both be provided")
		}
		endpoint["username"], endpoint["password"] = username, password
	} else if requiresCredentials {
		return nil, false, errors.New("OpenVPN profile requires username and password")
	}
	endpoint["servers"] = remotes
	endpoint["network"] = network
	tls["certificate"] = []string{blocks["ca"]}
	if blocks["cert"] != "" {
		tls["client_certificate"] = []string{blocks["cert"]}
		tls["client_key"] = []string{blocks["key"]}
	}
	wrapType := ""
	for _, candidate := range []string{"tls-auth", "tls-crypt", "tls-crypt-v2"} {
		if blocks[candidate] != "" {
			if wrapType != "" {
				return nil, false, errors.New("multiple OpenVPN TLS wrapping modes")
			}
			wrapType = candidate
		}
	}
	if wrapType != "" {
		wrap := map[string]any{"type": strings.ReplaceAll(wrapType, "-", "_"), "key": []string{blocks[wrapType]}}
		if value, ok := endpoint["key_direction"]; ok {
			if wrapType != "tls-auth" {
				return nil, false, errors.New("key-direction requires tls-auth")
			}
			wrap["direction"] = value
			delete(endpoint, "key_direction")
		}
		tls["control_wrap"] = wrap
	} else if _, ok := endpoint["key_direction"]; ok {
		return nil, false, errors.New("key-direction requires tls-auth")
	}
	endpoint["tls"] = tls
	return endpoint, usesHostname, nil
}

func openVPNProtocol(value string) (string, error) {
	switch strings.ToLower(value) {
	case "udp", "udp4", "udp6":
		return "udp", nil
	case "tcp", "tcp-client", "tcp4-client", "tcp6-client":
		return "tcp", nil
	default:
		return "", fmt.Errorf("unsupported OpenVPN protocol %q", value)
	}
}
