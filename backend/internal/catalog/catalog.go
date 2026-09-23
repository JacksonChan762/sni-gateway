package catalog

import (
	"compress/gzip"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"sync"
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
var snapshot Snapshot
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
		if err := json.Unmarshal(data, &snapshot); err != nil {
			loadErr = err
			return
		}
		if snapshot.SourceCommit == "" || len(snapshot.Providers) == 0 {
			loadErr = errors.New("empty provider catalog")
		}
	})
	return snapshot, loadErr
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
