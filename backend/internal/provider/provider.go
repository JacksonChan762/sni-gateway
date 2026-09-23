package provider

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"

	"sni-gateway/internal/models"
)

// Native names and WireGuard requirements follow Gluetun's built-in providers.
var native = map[string]bool{
	"airvpn": true, "cyberghost": true, "expressvpn": true, "fastestvpn": true,
	"giganews": true, "hidemyass": true, "ipvanish": true, "ivpn": true,
	"mullvad": true, "nordvpn": true, "privado": true, "private internet access": true,
	"privatevpn": true, "protonvpn": true, "purevpn": true, "slickvpn": true,
	"surfshark": true, "torguard": true, "vpnsecure": true, "vpn unlimited": true,
	"vyprvpn": true, "windscribe": true,
}
var wireguard = map[string]bool{
	"airvpn": true, "fastestvpn": true, "ivpn": true, "mullvad": true,
	"nordvpn": true, "protonvpn": true, "surfshark": true, "windscribe": true,
}
var ivpnAccountID = regexp.MustCompile(`^(i|ivpn)-[a-zA-Z0-9]{4}-[a-zA-Z0-9]{4}-[a-zA-Z0-9]{4}$`)

func Native(name string) bool            { return native[name] }
func WireGuard(name string) bool         { return wireguard[name] }
func NeedsAddress(name string) bool      { return name != "nordvpn" && name != "protonvpn" }
func NeedsPresharedKey(name string) bool { return name == "airvpn" }

// WireGuard ports and interface defaults follow the upstream Gluetun provider
// connection settings. The server catalog supplies the endpoint IP and peer key.
func WireGuardPort(name string) (uint16, bool) {
	ports := map[string]uint16{
		"airvpn": 1637, "fastestvpn": 51820, "ivpn": 58237,
		"mullvad": 51820, "nordvpn": 51820, "protonvpn": 51820,
		"surfshark": 51820, "windscribe": 1194,
	}
	port, ok := ports[name]
	return port, ok
}

func DefaultAddresses(name string) string {
	switch name {
	case "nordvpn":
		return "10.5.0.2/32"
	case "protonvpn":
		return "10.2.0.2/32,2a07:b944::2:2:0:0/128"
	default:
		return ""
	}
}
func NeedsClientCertificate(name string) bool {
	return name == "airvpn" || name == "vpn unlimited" || name == "cyberghost" || name == "vpnsecure"
}

type Credentials struct {
	WireguardPrivateKey   string `json:"wireguard_private_key"`
	WireguardAddresses    string `json:"wireguard_addresses"`
	WireguardPresharedKey string `json:"wireguard_preshared_key"`
	OpenVPNUser           string `json:"openvpn_user"`
	OpenVPNPassword       string `json:"openvpn_password"`
	OpenVPNCert           string `json:"openvpn_cert"`
	OpenVPNKey            string `json:"openvpn_key"`
	OpenVPNKeyPassphrase  string `json:"openvpn_key_passphrase"`
}

func Parse(raw string) (Credentials, error) {
	var credentials Credentials
	if err := json.Unmarshal([]byte(raw), &credentials); err != nil {
		return credentials, err
	}
	return credentials, nil
}

func Validate(exit models.Exit) error {
	if !Native(exit.Provider) {
		return fmt.Errorf("unsupported Gluetun provider %q", exit.Provider)
	}
	if strings.TrimSpace(exit.Country) == "" {
		return errors.New("choose a provider country")
	}
	credentials, err := Parse(exit.ConfigJSON)
	if err != nil {
		return err
	}
	switch exit.Protocol {
	case "wireguard":
		if !WireGuard(exit.Provider) {
			return fmt.Errorf("%s does not support WireGuard in Gluetun", exit.Provider)
		}
		if err := key(credentials.WireguardPrivateKey); err != nil {
			return fmt.Errorf("WireGuard private key: %w", err)
		}
		if NeedsAddress(exit.Provider) && strings.TrimSpace(credentials.WireguardAddresses) == "" {
			return errors.New("WireGuard tunnel address is required for this provider")
		}
		if credentials.WireguardAddresses != "" {
			for _, address := range strings.Split(credentials.WireguardAddresses, ",") {
				if _, err := netip.ParsePrefix(strings.TrimSpace(address)); err != nil {
					return errors.New("WireGuard tunnel addresses must be CIDR prefixes")
				}
			}
		}
		if NeedsPresharedKey(exit.Provider) && credentials.WireguardPresharedKey == "" {
			return errors.New("WireGuard preshared key is required for AirVPN")
		}
		if credentials.WireguardPresharedKey != "" {
			if err := key(credentials.WireguardPresharedKey); err != nil {
				return fmt.Errorf("WireGuard preshared key: %w", err)
			}
		}
	case "openvpn":
		needsUser := exit.Provider != "airvpn" && exit.Provider != "vpnsecure"
		if needsUser && credentials.OpenVPNUser == "" {
			return errors.New("OpenVPN username is required")
		}
		if needsUser && credentials.OpenVPNPassword == "" && !(exit.Provider == "ivpn" && ivpnAccountID.MatchString(credentials.OpenVPNUser)) {
			return errors.New("OpenVPN password is required")
		}
		if NeedsClientCertificate(exit.Provider) && (credentials.OpenVPNCert == "" || credentials.OpenVPNKey == "") {
			return errors.New("OpenVPN client certificate and key are required for this provider")
		}
		if (credentials.OpenVPNCert == "") != (credentials.OpenVPNKey == "") {
			return errors.New("OpenVPN client certificate and key must be provided together")
		}
		if exit.Provider == "vpnsecure" && credentials.OpenVPNKeyPassphrase == "" {
			return errors.New("VPN Secure encrypted OpenVPN key passphrase is required")
		}
	default:
		return errors.New("protocol must be wireguard or openvpn")
	}
	return nil
}

func key(value string) error {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(decoded) != 32 {
		return errors.New("must be a 32-byte base64 key")
	}
	return nil
}

func Env(exit models.Exit) (map[string]string, error) {
	if err := Validate(exit); err != nil {
		return nil, err
	}
	credentials, _ := Parse(exit.ConfigJSON)
	env := map[string]string{
		"VPN_SERVICE_PROVIDER": exit.Provider, "VPN_TYPE": exit.Protocol,
		"SERVER_COUNTRIES": exit.Country, "FIREWALL_INPUT_PORTS": "1080",
		"UPDATER_PERIOD": "480h",
	}
	if exit.City != "" {
		env["SERVER_CITIES"] = exit.City
	}
	if exit.Protocol == "wireguard" {
		env["WIREGUARD_PRIVATE_KEY"] = credentials.WireguardPrivateKey
		if credentials.WireguardAddresses != "" {
			env["WIREGUARD_ADDRESSES"] = credentials.WireguardAddresses
		}
		if credentials.WireguardPresharedKey != "" {
			env["WIREGUARD_PRESHARED_KEY"] = credentials.WireguardPresharedKey
		}
	} else {
		if credentials.OpenVPNUser != "" {
			env["OPENVPN_USER"] = credentials.OpenVPNUser
		}
		if credentials.OpenVPNPassword != "" {
			env["OPENVPN_PASSWORD"] = credentials.OpenVPNPassword
		}
		if credentials.OpenVPNCert != "" {
			env["OPENVPN_CERT"] = pemBody(credentials.OpenVPNCert)
		}
		if credentials.OpenVPNKey != "" {
			if strings.Contains(credentials.OpenVPNKey, "ENCRYPTED PRIVATE KEY") {
				env["OPENVPN_ENCRYPTED_KEY"] = pemBody(credentials.OpenVPNKey)
			} else {
				env["OPENVPN_KEY"] = pemBody(credentials.OpenVPNKey)
			}
		}
		if credentials.OpenVPNKeyPassphrase != "" {
			env["OPENVPN_KEY_PASSPHRASE"] = credentials.OpenVPNKeyPassphrase
		}
	}
	return env, nil
}

func pemBody(value string) string {
	var lines []string
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "-----") {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "")
}
