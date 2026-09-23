package catalog

import "testing"

func TestSnapshotSearchesAllProvidersWithoutLeakingOtherProtocols(t *testing.T) {
	providers, commit, err := ListProviders()
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 23 || len(commit) != 40 {
		t.Fatalf("catalog incomplete: %d providers, commit %q", len(providers), commit)
	}
	for _, provider := range providers {
		for _, protocol := range []string{"openvpn", "wireguard"} {
			count := provider.OpenVPN
			if protocol == "wireguard" {
				count = provider.WireGuard
			}
			if count == 0 {
				continue
			}
			page, err := Search(provider.Name, protocol, "", "", 0, 1)
			if err != nil {
				t.Fatal(err)
			}
			if page.Total != count || len(page.Servers) != 1 || page.Servers[0].Protocol != protocol || page.Servers[0].IP == "" {
				t.Fatalf("bad catalog result for %s/%s: %+v", provider.Name, protocol, page)
			}
		}
	}
}

func TestSearchPaginationAndValidation(t *testing.T) {
	first, err := Search("protonvpn", "openvpn", "Afghanistan", "Kabul", 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Search("protonvpn", "openvpn", "Afghanistan", "Kabul", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if first.Total < 2 || second.Total != first.Total || first.Servers[0].ID == second.Servers[0].ID {
		t.Fatalf("pagination failed: %+v %+v", first, second)
	}
	if _, err := Search("unknown", "openvpn", "", "", 0, 50); err == nil {
		t.Fatal("unknown provider accepted")
	}
	if _, err := Search("protonvpn", "openvpn", "", "", 0, 101); err == nil {
		t.Fatal("unbounded page accepted")
	}
}
