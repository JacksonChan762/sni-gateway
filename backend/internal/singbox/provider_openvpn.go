package singbox

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	"sni-gateway/internal/catalog"
	"sni-gateway/internal/models"
	"sni-gateway/internal/provider"

	"github.com/youmark/pkcs8"
)

func buildProviderOpenVPNProfile(exit models.Exit, clientCert, clientKey, passphrase string) (string, error) {
	server, err := catalog.Select(exit.Provider, "openvpn", exit.Country, exit.City, exit.Name)
	if err != nil {
		return "", err
	}
	network := "udp"
	if !server.UDP {
		network = "tcp"
	}
	if !server.UDP && !server.TCP {
		return "", errors.New("selected OpenVPN server has no supported transport")
	}
	port, err := provider.OpenVPNPort(exit.Provider, network)
	if err != nil {
		return "", err
	}
	lines, err := provider.OpenVPNTemplate(exit.Provider, network)
	if err != nil {
		return "", err
	}
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "remote "):
			lines[i] = fmt.Sprintf("remote %s %d", server.IP, port)
		case strings.HasPrefix(line, "verify-x509-name "):
			if server.Hostname == "" {
				return "", errors.New("selected OpenVPN server lacks a certificate name")
			}
			name := server.Hostname
			if strings.HasSuffix(line, " name-prefix") {
				name = strings.Split(name, ".")[0]
			}
			parts := strings.Fields(line)
			lines[i] = "verify-x509-name " + name + " " + parts[len(parts)-1]
		case strings.HasPrefix(line, "tls-cipher "):
			converted := compatibleTLSCiphers(strings.TrimPrefix(line, "tls-cipher "))
			if converted == "" {
				lines[i] = ""
			} else {
				lines[i] = "tls-cipher " + converted
			}
		}
	}
	if clientCert != "" || clientKey != "" {
		if clientCert == "" || clientKey == "" {
			return "", errors.New("OpenVPN certificate and key must both be provided")
		}
		cert, err := certificatePEM(clientCert)
		if err != nil {
			return "", err
		}
		key, err := privateKeyPEM(clientKey, passphrase)
		if err != nil {
			return "", err
		}
		lines = append(lines, "<cert>", cert, "</cert>", "<key>", key, "</key>")
	}
	return strings.Join(lines, "\n") + "\n", nil
}

// sing-box uses OpenSSL cipher names and its TLS engine does not implement
// legacy finite-field DHE/DSS suites. Keep only suites both sides advertise.
func compatibleTLSCiphers(value string) string {
	aliases := map[string]string{
		"TLS-ECDHE-RSA-WITH-AES-256-GCM-SHA384": "ECDHE-RSA-AES256-GCM-SHA384",
		"TLS-RSA-WITH-AES-256-CBC-SHA":          "AES256-SHA",
	}
	var accepted []string
	for _, item := range strings.Split(value, ":") {
		if name := aliases[item]; name != "" {
			accepted = append(accepted, name)
		}
	}
	return strings.Join(accepted, ":")
}

func certificatePEM(value string) (string, error) {
	if strings.Contains(value, "-----BEGIN CERTIFICATE-----") {
		return value, nil
	}
	der, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(value), ""))
	if err != nil {
		return "", fmt.Errorf("invalid OpenVPN certificate: %w", err)
	}
	if _, err := x509.ParseCertificate(der); err != nil {
		return "", fmt.Errorf("invalid OpenVPN certificate: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), nil
}

func privateKeyPEM(value, passphrase string) (string, error) {
	if strings.Contains(value, "-----BEGIN ") {
		block, _ := pem.Decode([]byte(value))
		if block == nil {
			return "", errors.New("invalid OpenVPN private key PEM")
		}
		if strings.Contains(block.Type, "ENCRYPTED") || x509.IsEncryptedPEMBlock(block) {
			return decryptOpenVPNKey(block, passphrase)
		}
		return value, nil
	}
	der, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(value), ""))
	if err != nil {
		return "", fmt.Errorf("invalid OpenVPN private key: %w", err)
	}
	if _, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), nil
	}
	if _, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})), nil
	}
	if _, err := x509.ParseECPrivateKey(der); err == nil {
		return string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})), nil
	}
	return decryptOpenVPNKey(&pem.Block{Type: "ENCRYPTED PRIVATE KEY", Bytes: der}, passphrase)
}

func decryptOpenVPNKey(block *pem.Block, passphrase string) (string, error) {
	if passphrase == "" {
		return "", errors.New("encrypted OpenVPN private key requires a passphrase")
	}
	var key any
	var err error
	if x509.IsEncryptedPEMBlock(block) {
		var der []byte
		der, err = x509.DecryptPEMBlock(block, []byte(passphrase))
		if err == nil {
			key, err = x509.ParsePKCS8PrivateKey(der)
			if err != nil {
				key, err = x509.ParsePKCS1PrivateKey(der)
			}
		}
	} else {
		key, err = pkcs8.ParsePKCS8PrivateKey(block.Bytes, []byte(passphrase))
	}
	if err != nil {
		return "", fmt.Errorf("could not decrypt OpenVPN private key: %w", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), nil
}
