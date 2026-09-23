package provider

import (
	"compress/gzip"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// Generated from Gluetun provider OpenVPN settings at
// e1c93c734e6922e47a88d4f6bca78b0cd33731e2 (MIT license).
// The profiles contain only public provider CA and TLS settings.
//
//go:embed data/openvpn_profiles.json.gz
var openVPNProfiles embed.FS

var profilesOnce sync.Once
var profiles map[string]map[string][]string
var profilesErr error

func openVPNTemplates() (map[string]map[string][]string, error) {
	profilesOnce.Do(func() {
		file, err := openVPNProfiles.Open("data/openvpn_profiles.json.gz")
		if err != nil {
			profilesErr = err
			return
		}
		defer file.Close()
		reader, err := gzip.NewReader(file)
		if err != nil {
			profilesErr = err
			return
		}
		defer reader.Close()
		profilesErr = json.NewDecoder(reader).Decode(&profiles)
	})
	return profiles, profilesErr
}

func OpenVPNTemplate(name, network string) ([]string, error) {
	all, err := openVPNTemplates()
	if err != nil {
		return nil, err
	}
	switch name {
	case "private internet access":
		name = "privateinternetaccess"
	case "vpn unlimited":
		name = "vpnunlimited"
	}
	variants, ok := all[name]
	if !ok {
		return nil, fmt.Errorf("no sing-box OpenVPN template for %s", name)
	}
	lines := variants[network]
	if len(lines) == 0 {
		return nil, errors.New("OpenVPN transport is unavailable for this provider")
	}
	return append([]string(nil), lines...), nil
}

func OpenVPNPort(name, network string) (uint16, error) {
	ports := map[string][2]uint16{
		"airvpn": {1194, 443}, "cyberghost": {443, 443}, "expressvpn": {1195, 0},
		"fastestvpn": {4443, 4443}, "giganews": {443, 0}, "hidemyass": {553, 8080},
		"ipvanish": {443, 0}, "ivpn": {1194, 443}, "nordvpn": {1194, 443},
		"privado": {1194, 1194}, "private internet access": {8080, 8443},
		"privatevpn": {1194, 443}, "protonvpn": {1194, 443}, "purevpn": {53, 80},
		"slickvpn": {443, 443}, "surfshark": {1194, 1443}, "torguard": {1912, 1912},
		"vpn unlimited": {1197, 1197}, "vpnsecure": {1282, 110},
		"vyprvpn": {443, 0}, "windscribe": {1194, 443},
	}
	portsForProvider, ok := ports[name]
	if !ok {
		return 0, fmt.Errorf("no OpenVPN port for %s", name)
	}
	if network == "udp" && portsForProvider[0] != 0 {
		return portsForProvider[0], nil
	}
	if network == "tcp" && portsForProvider[1] != 0 {
		return portsForProvider[1], nil
	}
	return 0, fmt.Errorf("%s does not support OpenVPN %s", name, strings.ToUpper(network))
}
