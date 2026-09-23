package catalog

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/netip"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const commitURL = "https://api.github.com/repos/qdm12/gluetun-servers/commits/main"
const archiveURL = "https://codeload.github.com/qdm12/gluetun-servers/tar.gz/"

var commitPattern = regexp.MustCompile(`^[a-f0-9]{40}$`)

type upstreamServer struct {
	VPN        string   `json:"vpn"`
	Country    string   `json:"country"`
	Region     string   `json:"region"`
	City       string   `json:"city"`
	ServerName string   `json:"server_name"`
	Hostname   string   `json:"hostname"`
	IPs        []string `json:"ips"`
	PublicKey  string   `json:"wgpubkey"`
	UDP        bool     `json:"udp"`
	TCP        bool     `json:"tcp"`
}

func LoadCache(filename string) error {
	if filename == "" {
		return nil
	}
	if _, err := Load(); err != nil {
		return err
	}
	file, err := os.Open(filename)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	reader, err := gzip.NewReader(io.LimitReader(file, 16<<20))
	if err != nil {
		return err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, 32<<20))
	if err != nil {
		return err
	}
	var cached Snapshot
	if err := json.Unmarshal(data, &cached); err != nil {
		return err
	}
	if err := validateSnapshot(cached); err != nil {
		return err
	}
	normalizeCountries(&cached)
	snapshot.Store(&cached)
	return nil
}

func StartUpdater(ctx context.Context, filename string, interval time.Duration) {
	if filename == "" || interval <= 0 {
		return
	}
	go func() {
		client := &http.Client{Timeout: 90 * time.Second}
		refresh := func() {
			current, err := Load()
			if err != nil {
				log.Printf("Provider catalog unavailable: %v", err)
				return
			}
			updated, err := fetchLatest(ctx, client, current.SourceCommit)
			if err != nil {
				log.Printf("Provider catalog update skipped: %v", err)
				return
			}
			if updated == nil {
				return
			}
			if err := saveSnapshot(filename, *updated); err != nil {
				log.Printf("Provider catalog update could not be saved: %v", err)
				return
			}
			snapshot.Store(updated)
			log.Printf("Provider catalog updated to %s (%d providers)", updated.SourceCommit, len(updated.Providers))
		}
		refresh()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refresh()
			}
		}
	}()
}

func fetchLatest(ctx context.Context, client *http.Client, currentCommit string) (*Snapshot, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, commitURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "sni-gateway-provider-catalog")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream commit request returned %d", response.StatusCode)
	}
	var commit struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&commit); err != nil {
		return nil, err
	}
	if !commitPattern.MatchString(commit.SHA) {
		return nil, errors.New("invalid upstream commit")
	}
	if commit.SHA == currentCommit {
		return nil, nil
	}
	request, err = http.NewRequestWithContext(ctx, http.MethodGet, archiveURL+commit.SHA, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "sni-gateway-provider-catalog")
	archive, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer archive.Body.Close()
	if archive.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream archive request returned %d", archive.StatusCode)
	}
	updated, err := parseArchive(io.LimitReader(archive.Body, 16<<20), commit.SHA)
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

func parseArchive(body io.Reader, commit string) (Snapshot, error) {
	compressed, err := gzip.NewReader(body)
	if err != nil {
		return Snapshot{}, err
	}
	defer compressed.Close()
	archive := tar.NewReader(compressed)
	result := Snapshot{SourceCommit: commit, Providers: map[string][]Server{}}
	count := 0
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Snapshot{}, err
		}
		if header.Typeflag != tar.TypeReg || !strings.Contains(header.Name, "/pkg/servers/") || path.Ext(header.Name) != ".json" || path.Base(header.Name) == "manifest.json" {
			continue
		}
		if header.Size < 0 || header.Size > 8<<20 {
			return Snapshot{}, errors.New("upstream provider file too large")
		}
		provider := strings.TrimSuffix(path.Base(header.Name), ".json")
		if provider == "" || strings.ContainsAny(provider, `/\\`) {
			return Snapshot{}, errors.New("invalid provider name")
		}
		data, err := io.ReadAll(io.LimitReader(archive, header.Size))
		if err != nil {
			return Snapshot{}, err
		}
		var source struct {
			Servers []upstreamServer `json:"servers"`
		}
		if err := json.Unmarshal(data, &source); err != nil {
			return Snapshot{}, fmt.Errorf("%s: %w", provider, err)
		}
		servers := make([]Server, 0, len(source.Servers))
		for _, raw := range source.Servers {
			if raw.VPN != "wireguard" && raw.VPN != "openvpn" {
				continue
			}
			ip := ""
			for _, candidate := range raw.IPs {
				if parsed, err := netip.ParseAddr(candidate); err == nil {
					ip = parsed.String()
					break
				}
			}
			if ip == "" {
				continue
			}
			name := raw.ServerName
			if name == "" {
				name = raw.Hostname
			}
			key := ""
			if raw.VPN == "wireguard" {
				key = raw.PublicKey
			}
			servers = append(servers, Server{ID: len(servers), Protocol: raw.VPN, Country: raw.Country, Region: raw.Region, City: raw.City, Name: name, Hostname: raw.Hostname, IP: ip, PublicKey: key, UDP: raw.UDP, TCP: raw.TCP})
		}
		result.Providers[provider] = servers
		count += len(servers)
	}
	if len(result.Providers) < 20 || count < 10000 {
		return Snapshot{}, errors.New("upstream provider catalog is incomplete")
	}
	if err := validateSnapshot(result); err != nil {
		return Snapshot{}, err
	}
	normalizeCountries(&result)
	return result, nil
}

func validateSnapshot(data Snapshot) error {
	if !commitPattern.MatchString(data.SourceCommit) || len(data.Providers) < 20 {
		return errors.New("invalid provider catalog")
	}
	count := 0
	for name, servers := range data.Providers {
		if name == "" || len(servers) == 0 {
			return errors.New("invalid provider entry")
		}
		for _, server := range servers {
			if server.Protocol != "openvpn" && server.Protocol != "wireguard" {
				return errors.New("invalid catalog protocol")
			}
			if _, err := netip.ParseAddr(server.IP); err != nil {
				return errors.New("invalid catalog server IP")
			}
		}
		count += len(servers)
	}
	if count < 10000 {
		return errors.New("provider catalog has too few servers")
	}
	return nil
}

func saveSnapshot(filename string, data Snapshot) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(filename), ".providers-*.gz")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err := temp.Chmod(0600); err != nil {
		temp.Close()
		return err
	}
	writer := gzip.NewWriter(temp)
	if err := json.NewEncoder(writer).Encode(data); err != nil {
		writer.Close()
		temp.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), filename)
}
