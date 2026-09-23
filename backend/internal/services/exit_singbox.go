package services

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/netip"
	"sort"
	"strings"
	"time"

	"sni-gateway/internal/catalog"
	"sni-gateway/internal/db"
	"sni-gateway/internal/gateway"
	"sni-gateway/internal/models"
	"sni-gateway/internal/singbox"
)

func (em *ExitManager) createAndStartSingBox(exit *models.Exit) error {
	if err := em.ensureDockerAvailable(); err != nil {
		return err
	}
	if autoServerExit(exit) {
		return em.createAndStartAutoSingBox(exit)
	}
	config, err := singbox.BuildExitConfig(*exit)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	em.logEvent(exit.ID, "start", "started", "starting sing-box")
	if err := em.docker.CreateSingBoxContainer(ctx, exit.Name, config); err != nil {
		em.updateStatus(exit, "unhealthy")
		em.logEvent(exit.ID, "start", "failed", err.Error())
		return err
	}
	var ip string
	if exit.Provider != "custom" {
		ip, err = probeProviderPublicIP(ctx, exit.Name)
	} else {
		ip, err = probeSingBoxPublicIP(exit.Name)
	}
	if err != nil {
		exit.Status, exit.PublicIP = "unhealthy", ""
		if dbErr := db.DB.Model(exit).Updates(map[string]any{"status": exit.Status, "public_ip": ""}).Error; dbErr != nil {
			return dbErr
		}
		em.logEvent(exit.ID, "start", "failed", "sing-box started but VPN probe failed: "+err.Error())
		return nil
	}
	exit.Status, exit.PublicIP = "running", ip
	if err := db.DB.Model(exit).Updates(map[string]any{"status": exit.Status, "public_ip": ip}).Error; err != nil {
		return err
	}
	em.logEvent(exit.ID, "start", "success", "VPN connected; public IP "+ip)
	return nil
}

const maxEndpointTrials = 4

type measuredEndpoint struct {
	endpointIP string
	publicIP   string
	config     []byte
	latency    time.Duration
}

func autoServerExit(exit *models.Exit) bool {
	config := parseConfigJSON(exit.ConfigJSON)
	switch value := config["auto_server"].(type) {
	case bool:
		return value
	case string:
		return strings.EqualFold(strings.TrimSpace(value), "true")
	}
	return false
}

// Include the last successful endpoint so it can win again. Sample other
// endpoints on every connection and bound the work for large countries.
func endpointTrials(candidates []catalog.Server, previous string) []catalog.Server {
	if len(candidates) == 0 {
		return nil
	}
	limit := min(maxEndpointTrials, len(candidates))
	result := make([]catalog.Server, 0, limit)
	for _, server := range candidates {
		if server.IP == previous {
			result = append(result, server)
			break
		}
	}
	for _, index := range rand.Perm(len(candidates)) {
		if len(result) == limit {
			break
		}
		server := candidates[index]
		if server.IP != previous {
			result = append(result, server)
		}
	}
	return result
}

func (em *ExitManager) createAndStartAutoSingBox(exit *models.Exit) error {
	candidates, err := catalog.Candidates(exit.Provider, exit.Protocol, exit.Country, exit.City)
	if err != nil {
		return err
	}
	trials := endpointTrials(candidates, exit.EndpointIP)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	em.logEvent(exit.ID, "start", "started", "testing VPN endpoints")

	var measured []measuredEndpoint
	var activeEndpoint string
	var lastErr error
	for _, server := range trials {
		candidateExit := *exit
		candidateExit.EndpointIP = server.IP
		config, buildErr := singbox.BuildExitConfig(candidateExit)
		if buildErr != nil {
			lastErr = buildErr
			continue
		}
		start := time.Now()
		if err = em.docker.CreateSingBoxContainer(ctx, exit.Name, config); err != nil {
			lastErr = err
			activeEndpoint = ""
			continue
		}
		activeEndpoint = server.IP
		ip, probeErr := probeProviderPublicIPFor(ctx, exit.Name, 40*time.Second)
		if probeErr != nil {
			lastErr = probeErr
			continue
		}
		measured = append(measured, measuredEndpoint{endpointIP: server.IP, publicIP: ip, config: config, latency: time.Since(start)})
	}
	if len(measured) == 0 {
		exit.Status, exit.PublicIP = "unhealthy", ""
		if dbErr := db.DB.Model(exit).Updates(map[string]any{"status": exit.Status, "public_ip": ""}).Error; dbErr != nil {
			return dbErr
		}
		em.logEvent(exit.ID, "start", "failed", fmt.Sprintf("no VPN endpoint passed the probe: %v", lastErr))
		return nil
	}
	sort.Slice(measured, func(i, j int) bool { return measured[i].latency < measured[j].latency })
	var chosen *measuredEndpoint
	for i := range measured {
		choice := &measured[i]
		if activeEndpoint == choice.endpointIP {
			chosen = choice
			break
		}
		if err = em.docker.CreateSingBoxContainer(ctx, exit.Name, choice.config); err != nil {
			lastErr = err
			activeEndpoint = ""
			continue
		}
		activeEndpoint = choice.endpointIP
		ip, probeErr := probeProviderPublicIPFor(ctx, exit.Name, 40*time.Second)
		if probeErr != nil {
			lastErr = probeErr
			continue
		}
		choice.publicIP = ip
		chosen = choice
		break
	}
	if chosen == nil {
		exit.Status, exit.PublicIP = "unhealthy", ""
		if dbErr := db.DB.Model(exit).Updates(map[string]any{"status": exit.Status, "public_ip": ""}).Error; dbErr != nil {
			return dbErr
		}
		em.logEvent(exit.ID, "start", "failed", fmt.Sprintf("measured endpoints could not reconnect: %v", lastErr))
		return nil
	}
	exit.Status, exit.PublicIP, exit.EndpointIP = "running", chosen.publicIP, chosen.endpointIP
	if err := db.DB.Model(exit).Updates(map[string]any{"status": exit.Status, "public_ip": chosen.publicIP, "endpoint_ip": chosen.endpointIP}).Error; err != nil {
		return err
	}
	em.logEvent(exit.ID, "start", "success", fmt.Sprintf("VPN connected; endpoint %s; public IP %s; probe %s", chosen.endpointIP, chosen.publicIP, chosen.latency.Round(time.Millisecond)))
	return nil
}

func (em *ExitManager) reconcileSingBox(before, after *models.Exit) error {
	defer em.lockExit(after.ID)()
	if !exitRuntimeConfigChanged(before, after) && before.Enabled == after.Enabled {
		return nil
	}
	if err := em.ensureDockerAvailable(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if before.Runtime == "gluetun" {
		if err := em.removeProviderContainers(ctx, before.Name); err != nil {
			return err
		}
	} else if before.Runtime != "sing-box" {
		if err := removeExitContainers(ctx, em.docker, before.Name); err != nil {
			return err
		}
	}
	if before.Name != after.Name || before.Runtime != "sing-box" {
		if err := em.docker.RemoveContainer(ctx, "singbox-"+before.Name, true); err != nil {
			return err
		}
	}
	if !after.Enabled {
		if err := em.docker.RemoveContainer(ctx, "singbox-"+after.Name, true); err != nil {
			return err
		}
		em.updateStatus(after, "stopped")
		return nil
	}
	return em.createAndStartSingBox(after)
}

func (em *ExitManager) stopSingBox(ctx context.Context, exit *models.Exit) error {
	if err := em.ensureDockerAvailable(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if err := em.docker.StopContainer(ctx, "singbox-"+exit.Name); err != nil {
		return err
	}
	exit.Status, exit.PublicIP = "stopped", ""
	return db.DB.Model(exit).Updates(map[string]any{"status": "stopped", "public_ip": ""}).Error
}

func (em *ExitManager) deleteSingBox(exit *models.Exit) error {
	if err := em.ensureDockerAvailable(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	return em.docker.RemoveContainer(ctx, "singbox-"+exit.Name, true)
}

func (em *ExitManager) getSingBoxLogs(exit *models.Exit, tail int) (string, error) {
	if err := em.ensureDockerAvailable(); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return em.docker.GetContainerLogs(ctx, "singbox-"+exit.Name, tail)
}

func (em *ExitManager) getSingBoxStatus(exit *models.Exit) (string, string, error) {
	if err := em.ensureDockerAvailable(); err != nil {
		return "unavailable", "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	status, err := em.docker.GetContainerStatus(ctx, "singbox-"+exit.Name)
	if err != nil {
		return "unknown", "", err
	}
	ip := ""
	switch status {
	case "running":
		ip, err = probeSingBoxPublicIP(exit.Name)
		if err != nil {
			status = "unhealthy"
		}
	case "exited", "dead":
		status = "stopped"
	}
	if err := db.DB.Model(exit).Updates(map[string]any{"status": status, "public_ip": ip}).Error; err != nil {
		return status, ip, err
	}
	return status, ip, nil
}

func probeSingBoxPublicIP(name string) (string, error) {
	return probeSingBoxPublicIPContext(context.Background(), name)
}

func probeSingBoxPublicIPContext(ctx context.Context, name string) (string, error) {
	transport := &http.Transport{DisableKeepAlives: true, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return gateway.DialSOCKSContext(ctx, "singbox-"+name+":1080", "api.ipify.org", 443)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 10 * time.Second, Transport: transport}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.ipify.org", nil)
	if err != nil {
		return "", err
	}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("public IP probe returned %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 128))
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(string(data))
	if _, err := netip.ParseAddr(ip); err != nil {
		return "", fmt.Errorf("invalid public IP response")
	}
	return ip, nil
}
