package main

import (
	"encoding/json"
	"fmt"
	"github.com/qdm12/gluetun/internal/configuration/settings"
	"github.com/qdm12/gluetun/internal/models"
	"github.com/qdm12/gluetun/internal/provider/airvpn"
	"github.com/qdm12/gluetun/internal/provider/cyberghost"
	"github.com/qdm12/gluetun/internal/provider/expressvpn"
	"github.com/qdm12/gluetun/internal/provider/fastestvpn"
	"github.com/qdm12/gluetun/internal/provider/giganews"
	"github.com/qdm12/gluetun/internal/provider/hidemyass"
	"github.com/qdm12/gluetun/internal/provider/ipvanish"
	"github.com/qdm12/gluetun/internal/provider/ivpn"
	"github.com/qdm12/gluetun/internal/provider/nordvpn"
	"github.com/qdm12/gluetun/internal/provider/privado"
	"github.com/qdm12/gluetun/internal/provider/privateinternetaccess"
	"github.com/qdm12/gluetun/internal/provider/privatevpn"
	"github.com/qdm12/gluetun/internal/provider/protonvpn"
	"github.com/qdm12/gluetun/internal/provider/purevpn"
	"github.com/qdm12/gluetun/internal/provider/slickvpn"
	"github.com/qdm12/gluetun/internal/provider/surfshark"
	"github.com/qdm12/gluetun/internal/provider/torguard"
	"github.com/qdm12/gluetun/internal/provider/vpnsecure"
	"github.com/qdm12/gluetun/internal/provider/vpnunlimited"
	"github.com/qdm12/gluetun/internal/provider/vyprvpn"
	"github.com/qdm12/gluetun/internal/provider/windscribe"
	"net/netip"
	"os"
)

type builder interface {
	OpenVPNConfig(models.Connection, settings.OpenVPN, bool) []string
}

func dump(name string) (out map[string][]string) {
	defer func() {
		if v := recover(); v != nil {
			fmt.Fprintln(os.Stderr, name, v)
		}
	}()
	var p builder
	switch name {
	case "airvpn":
		p = &airvpn.Provider{}
	case "cyberghost":
		p = &cyberghost.Provider{}
	case "expressvpn":
		p = &expressvpn.Provider{}
	case "fastestvpn":
		p = &fastestvpn.Provider{}
	case "giganews":
		p = &giganews.Provider{}
	case "hidemyass":
		p = &hidemyass.Provider{}
	case "ipvanish":
		p = &ipvanish.Provider{}
	case "ivpn":
		p = &ivpn.Provider{}
	case "nordvpn":
		p = &nordvpn.Provider{}
	case "privado":
		p = &privado.Provider{}
	case "privateinternetaccess":
		p = &privateinternetaccess.Provider{}
	case "privatevpn":
		p = &privatevpn.Provider{}
	case "protonvpn":
		p = &protonvpn.Provider{}
	case "purevpn":
		p = &purevpn.Provider{}
	case "slickvpn":
		p = &slickvpn.Provider{}
	case "surfshark":
		p = &surfshark.Provider{}
	case "torguard":
		p = &torguard.Provider{}
	case "vpnunlimited":
		p = &vpnunlimited.Provider{}
	case "vpnsecure":
		p = &vpnsecure.Provider{}
	case "vyprvpn":
		p = &vyprvpn.Provider{}
	case "windscribe":
		p = &windscribe.Provider{}
	}
	u, pass, auth, cert, key, encrypted, passphrase, preset := "", "", "", "", "", "", "", "strong"
	verbosity := 0
	mss := uint16(0)
	s := settings.OpenVPN{Version: "2.6", User: &u, Password: &pass, Auth: &auth, Cert: &cert, Key: &key, EncryptedKey: &encrypted, KeyPassphrase: &passphrase, PIAEncPreset: &preset, MSSFix: &mss, Interface: "tun0", ProcessUser: "root", Verbosity: &verbosity}
	out = make(map[string][]string)
	for _, proto := range []string{"udp", "tcp"} {
		conn := models.Connection{Type: "openvpn", IP: netip.MustParseAddr("203.0.113.10"), Port: 1194, Protocol: proto, Hostname: "example.com", ServerName: "example"}
		out[proto] = p.OpenVPNConfig(conn, s, false)
	}
	return out
}
func main() {
	all := make(map[string]map[string][]string)
	for _, name := range []string{"airvpn", "cyberghost", "expressvpn", "fastestvpn", "giganews", "hidemyass", "ipvanish", "ivpn", "nordvpn", "privado", "privateinternetaccess", "privatevpn", "protonvpn", "purevpn", "slickvpn", "surfshark", "torguard", "vpnunlimited", "vpnsecure", "vyprvpn", "windscribe"} {
		if value := dump(name); value != nil {
			all[name] = value
		}
	}
	if err := json.NewEncoder(os.Stdout).Encode(all); err != nil {
		panic(err)
	}
}
