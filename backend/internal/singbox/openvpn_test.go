package singbox

import (
	"encoding/json"
	"strings"
	"testing"

	"sni-gateway/internal/models"
)

const sampleOpenVPN = "client\nproto tcp-client\nremote vpn.example.test 443\nremote-cert-tls server\n<ca>\nTEST CA\n</ca>\n"

func TestBuildOpenVPNExitRoutesThroughVPN(t *testing.T) {
	raw, _ := json.Marshal(map[string]string{
		"openvpn_profile": sampleOpenVPN, "openvpn_user": "alice", "openvpn_password": "secret",
	})
	data, err := BuildExitConfig(models.Exit{Protocol: "openvpn", ConfigJSON: string(raw)})
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	endpoint := config["endpoints"].([]any)[0].(map[string]any)
	if endpoint["type"] != "openvpn-client" || endpoint["network"] != "tcp" || endpoint["username"] != "alice" || endpoint["password"] != "secret" || endpoint["domain_resolver"] != "bootstrap" {
		t.Fatalf("unexpected OpenVPN endpoint: %v", endpoint)
	}
	if config["route"].(map[string]any)["final"] != "vpn" {
		t.Fatal("destination route must use VPN")
	}
	dns := config["dns"].(map[string]any)["servers"].([]any)[0].(map[string]any)
	if dns["detour"] != "vpn" {
		t.Fatal("destination DNS must use VPN")
	}
}

func TestOpenVPNProfileRejectsUnsupportedOrIncompleteConfig(t *testing.T) {
	cases := []struct{ profile, username, password, want string }{
		{strings.Replace(sampleOpenVPN, "<ca>\nTEST CA\n</ca>", "", 1), "", "", "inline <ca>"},
		{sampleOpenVPN + "script-security 2\n", "", "", "unsupported OpenVPN option"},
		{sampleOpenVPN + "ca /etc/openvpn/ca.crt\n", "", "", "external ca files"},
		{sampleOpenVPN + "key-direction 1\n", "", "", "requires tls-auth"},
		{sampleOpenVPN + "auth-user-pass\n", "", "", "requires username and password"},
		{sampleOpenVPN + "dev tap\n", "", "", "only tun devices"},
		{sampleOpenVPN, "alice", "", "username and password"},
	}
	for _, tc := range cases {
		_, _, err := parseOpenVPNProfile(tc.profile, tc.username, tc.password)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("wanted %q, got %v", tc.want, err)
		}
	}
}
