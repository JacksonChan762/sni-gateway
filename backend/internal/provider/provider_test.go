package provider

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"sni-gateway/internal/models"
)

func testExit(name, protocol string, credentials Credentials) models.Exit {
	raw, _ := json.Marshal(credentials)
	return models.Exit{Name: "test", Runtime: "gluetun", Provider: name, Protocol: protocol, Country: "Netherlands", ConfigJSON: string(raw)}
}

func TestProviderOpenVPNUsesCredentialsWithoutProfile(t *testing.T) {
	exit := testExit("protonvpn", "openvpn", Credentials{OpenVPNUser: "service-user", OpenVPNPassword: "service-pass"})
	env, err := Env(exit)
	if err != nil {
		t.Fatal(err)
	}
	if env["OPENVPN_USER"] != "service-user" || env["OPENVPN_PASSWORD"] != "service-pass" || env["UPDATER_PERIOD"] != "480h" || env["FIREWALL_INPUT_PORTS"] != "1080" {
		t.Fatalf("wrong provider env: %+v", env)
	}
	if _, ok := env["OPENVPN_PROFILE"]; ok {
		t.Fatal("unexpected profile")
	}
}

func TestProviderWireguardRequirements(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	if err := Validate(testExit("protonvpn", "wireguard", Credentials{WireguardPrivateKey: key})); err != nil {
		t.Fatal(err)
	}
	if err := Validate(testExit("mullvad", "wireguard", Credentials{WireguardPrivateKey: key})); err == nil {
		t.Fatal("Mullvad address was not required")
	}
	if err := Validate(testExit("airvpn", "wireguard", Credentials{WireguardPrivateKey: key, WireguardAddresses: "10.2.0.2/32"})); err == nil {
		t.Fatal("AirVPN preshared key was not required")
	}
	if err := Validate(testExit("cyberghost", "wireguard", Credentials{WireguardPrivateKey: key})); err == nil {
		t.Fatal("unsupported protocol accepted")
	}
}

func TestProviderClientCertificate(t *testing.T) {
	exit := testExit("cyberghost", "openvpn", Credentials{OpenVPNUser: "u", OpenVPNPassword: "p"})
	if err := Validate(exit); err == nil {
		t.Fatal("missing certificate accepted")
	}
	exit = testExit("cyberghost", "openvpn", Credentials{OpenVPNUser: "u", OpenVPNPassword: "p", OpenVPNCert: "-----BEGIN CERTIFICATE-----\nQUJD\n-----END CERTIFICATE-----", OpenVPNKey: "-----BEGIN PRIVATE KEY-----\nREVG\n-----END PRIVATE KEY-----"})
	env, err := Env(exit)
	if err != nil {
		t.Fatal(err)
	}
	if env["OPENVPN_CERT"] != "QUJD" || env["OPENVPN_KEY"] != "REVG" {
		t.Fatal("PEM conversion failed")
	}
}

func TestOpenVPNProviderSpecificAuthentication(t *testing.T) {
	cert := "-----BEGIN CERTIFICATE-----\nQUJD\n-----END CERTIFICATE-----"
	key := "-----BEGIN PRIVATE KEY-----\nREVG\n-----END PRIVATE KEY-----"
	for _, test := range []struct {
		name        string
		credentials Credentials
		valid       bool
	}{
		{"airvpn", Credentials{OpenVPNCert: cert, OpenVPNKey: key}, true},
		{"vpn unlimited", Credentials{OpenVPNCert: cert, OpenVPNKey: key}, false},
		{"vpn unlimited", Credentials{OpenVPNUser: "u", OpenVPNPassword: "p", OpenVPNCert: cert, OpenVPNKey: key}, true},
		{"vpnsecure", Credentials{OpenVPNCert: cert, OpenVPNKey: key}, false},
		{"vpnsecure", Credentials{OpenVPNCert: cert, OpenVPNKey: key, OpenVPNKeyPassphrase: "pass"}, true},
		{"ivpn", Credentials{OpenVPNUser: "ivpn-ABCD-EFGH-IJKL"}, true},
	} {
		err := Validate(testExit(test.name, "openvpn", test.credentials))
		if (err == nil) != test.valid {
			t.Errorf("%s valid=%v: %v", test.name, test.valid, err)
		}
	}
}
