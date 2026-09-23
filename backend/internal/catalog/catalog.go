package catalog

import (
	"compress/gzip"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

//go:embed data/servers.json.gz
var embedded embed.FS

type Server struct {
	ID        int    `json:"id"`
	Protocol  string `json:"protocol"`
	Country   string `json:"country"`
	Region    string `json:"region"`
	City      string `json:"city"`
	Name      string `json:"name"`
	Hostname  string `json:"hostname"`
	IP        string `json:"ip"`
	PublicKey string `json:"public_key,omitempty"`
	UDP       bool   `json:"udp"`
	TCP       bool   `json:"tcp"`
}

type Provider struct {
	Name      string   `json:"name"`
	OpenVPN   int      `json:"openvpn"`
	WireGuard int      `json:"wireguard"`
	Countries []string `json:"countries"`
}

type Snapshot struct {
	SourceCommit string              `json:"source_commit"`
	Providers    map[string][]Server `json:"providers"`
}

type Page struct {
	SourceCommit string   `json:"source_commit"`
	Total        int      `json:"total"`
	Servers      []Server `json:"servers"`
}

var once sync.Once
var snapshot atomic.Pointer[Snapshot]
var loadErr error

func Load() (Snapshot, error) {
	once.Do(func() {
		file, err := embedded.Open("data/servers.json.gz")
		if err != nil {
			loadErr = err
			return
		}
		defer file.Close()
		reader, err := gzip.NewReader(file)
		if err != nil {
			loadErr = err
			return
		}
		defer reader.Close()
		data, err := io.ReadAll(io.LimitReader(reader, 32<<20))
		if err != nil {
			loadErr = err
			return
		}
		var initial Snapshot
		if err := json.Unmarshal(data, &initial); err != nil {
			loadErr = err
			return
		}
		if initial.SourceCommit == "" || len(initial.Providers) == 0 {
			loadErr = errors.New("empty provider catalog")
			return
		}
		snapshot.Store(&initial)
	})
	if loadErr != nil {
		return Snapshot{}, loadErr
	}
	return *snapshot.Load(), nil
}

func ListProviders() ([]Provider, string, error) {
	data, err := Load()
	if err != nil {
		return nil, "", err
	}
	result := make([]Provider, 0, len(data.Providers))
	for name, servers := range data.Providers {
		provider := Provider{Name: name, Countries: []string{}}
		countries := make(map[string]bool)
		for _, server := range servers {
			if server.Protocol == "wireguard" {
				provider.WireGuard++
			} else {
				provider.OpenVPN++
			}
			if server.Country != "" {
				countries[server.Country] = true
			}
		}
		for country := range countries {
			provider.Countries = append(provider.Countries, country)
		}
		sort.Strings(provider.Countries)
		result = append(result, provider)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, data.SourceCommit, nil
}

func Search(provider, protocol, country, query string, offset, limit int) (Page, error) {
	data, err := Load()
	if err != nil {
		return Page{}, err
	}
	servers, ok := data.Providers[provider]
	if !ok {
		return Page{}, errors.New("unknown VPN provider")
	}
	if protocol != "openvpn" && protocol != "wireguard" {
		return Page{}, errors.New("protocol must be openvpn or wireguard")
	}
	if offset < 0 || limit < 1 || limit > 100 {
		return Page{}, errors.New("invalid pagination")
	}
	result := Page{SourceCommit: data.SourceCommit, Servers: []Server{}}
	country, query = strings.ToLower(strings.TrimSpace(country)), strings.ToLower(strings.TrimSpace(query))
	for _, server := range servers {
		if server.Protocol != protocol || country != "" && strings.ToLower(server.Country) != country {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(server.Name+" "+server.Hostname+" "+server.City+" "+server.Region+" "+server.IP), query) {
			continue
		}
		if result.Total >= offset && len(result.Servers) < limit {
			result.Servers = append(result.Servers, server)
		}
		result.Total++
	}
	return result, nil
}

type Location struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type Locations struct {
	SourceCommit string     `json:"source_commit"`
	Countries    []Location `json:"countries"`
	Cities       []Location `json:"cities"`
}

func ListLocations(provider, protocol, country string) (Locations, error) {
	data, err := Load()
	if err != nil {
		return Locations{}, err
	}
	servers, ok := data.Providers[provider]
	if !ok {
		return Locations{}, errors.New("unknown VPN provider")
	}
	if protocol != "openvpn" && protocol != "wireguard" {
		return Locations{}, errors.New("protocol must be openvpn or wireguard")
	}
	result := Locations{SourceCommit: data.SourceCommit, Countries: []Location{}, Cities: []Location{}}
	countries, cities := map[string]int{}, map[string]int{}
	for _, server := range servers {
		if server.Protocol != protocol {
			continue
		}
		if server.Country != "" {
			countries[server.Country]++
		}
		if country == "" || strings.EqualFold(server.Country, country) {
			if server.City != "" {
				cities[server.City]++
			}
		}
	}
	for name, count := range countries {
		result.Countries = append(result.Countries, Location{name, count})
	}
	for name, count := range cities {
		result.Cities = append(result.Cities, Location{name, count})
	}
	sort.Slice(result.Countries, func(i, j int) bool { return result.Countries[i].Name < result.Countries[j].Name })
	sort.Slice(result.Cities, func(i, j int) bool { return result.Cities[i].Name < result.Cities[j].Name })
	return result, nil
}

func Select(provider, protocol, country, city, seed string, transport ...string) (Server, error) {
	data, err := Load()
	if err != nil {
		return Server{}, err
	}
	servers, ok := data.Providers[provider]
	if !ok {
		return Server{}, errors.New("unknown VPN provider")
	}
	if protocol != "openvpn" && protocol != "wireguard" {
		return Server{}, errors.New("protocol must be openvpn or wireguard")
	}
	if country == "" {
		return Server{}, errors.New("select a VPN country")
	}
	var candidates []Server
	for _, server := range servers {
		if server.Protocol != protocol || !strings.EqualFold(server.Country, country) {
			continue
		}
		if city != "" && !strings.EqualFold(server.City, city) {
			continue
		}
		if protocol == "wireguard" && server.PublicKey == "" {
			continue
		}
		if len(transport) > 0 && transport[0] == "udp" && !server.UDP {
			continue
		}
		if len(transport) > 0 && transport[0] == "tcp" && !server.TCP {
			continue
		}
		if _, err := netip.ParseAddr(server.IP); err != nil {
			continue
		}
		candidates = append(candidates, server)
	}
	if len(candidates) == 0 {
		return Server{}, errors.New("no compatible VPN server for provider and location")
	}
	// A stable choice avoids changing the endpoint between validation and start.
	var hash uint32 = 2166136261
	for _, b := range []byte(seed) {
		hash = (hash ^ uint32(b)) * 16777619
	}
	return candidates[int(hash%uint32(len(candidates)))], nil
}
