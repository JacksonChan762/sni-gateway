package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"sort"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-units"
)

const VPNNetwork = "vpn-net"

type Client struct {
	cli *client.Client
}

func (c *Client) ensureImage(ctx context.Context, imageRef string) error {
	if _, _, err := c.cli.ImageInspectWithRaw(ctx, imageRef); err == nil {
		return nil
	}

	rc, err := c.cli.ImagePull(ctx, imageRef, types.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer rc.Close()

	_, err = io.Copy(io.Discard, rc)
	return err
}

func NewClient(socketPath string) (*Client, error) {
	cli, err := client.NewClientWithOpts(
		client.WithHost("unix://"+socketPath),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, err
	}
	return &Client{cli: cli}, nil
}

type GluetunConfig struct {
	Name            string
	Protocol        string // wireguard / openvpn
	Provider        string
	Country         string
	City            string
	RelayPort       int
	WireguardKey    string
	WireguardPubKey string
	WireguardAddr   string
	OpenVPNUser     string
	OpenVPNPass     string
	ExtraEnv        map[string]string
}

type RelayConfig struct {
	Name       string
	ListenPort int
}

const relayProfile = "low-latency-v1"
const containerRunningTimeout = 45 * time.Second
const containerRunningPollInterval = 500 * time.Millisecond
const containerRunningStableDuration = 2 * time.Second

func (c *Client) CreateGluetunContainer(ctx context.Context, cfg GluetunConfig) error {
	containerName := "gluetun-" + cfg.Name
	imageRef := "qmcgaw/gluetun:latest"

	env := orderedGluetunEnv(buildGluetunEnvMap(cfg), cfg.Protocol)

	_ = c.RemoveContainer(ctx, containerName, true)
	if err := c.ensureImage(ctx, imageRef); err != nil {
		return fmt.Errorf("ensure gluetun image: %w", err)
	}

	resp, err := c.cli.ContainerCreate(ctx,
		&container.Config{
			Image: imageRef,
			Env:   env,
		},
		&container.HostConfig{
			CapAdd: []string{"NET_ADMIN"},
			Resources: container.Resources{
				Devices: []container.DeviceMapping{
					{PathOnHost: "/dev/net/tun", PathInContainer: "/dev/net/tun", CgroupPermissions: "rwm"},
				},
			},
			Sysctls:       map[string]string{"net.ipv6.conf.all.disable_ipv6": "1"},
			NetworkMode:   container.NetworkMode(VPNNetwork),
			RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
		},
		&network.NetworkingConfig{},
		nil,
		containerName,
	)
	if err != nil {
		return fmt.Errorf("create gluetun container: %w", err)
	}

	if err := c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return err
	}

	if err := c.WaitForContainerRunning(ctx, containerName); err != nil {
		return c.errorWithContainerLogs(ctx, containerName, err)
	}

	return nil
}

func buildGluetunEnvMap(cfg GluetunConfig) map[string]string {
	envMap := make(map[string]string, len(cfg.ExtraEnv)+8)
	for key, value := range cfg.ExtraEnv {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			envMap[key] = value
		}
	}
	envMap["VPN_SERVICE_PROVIDER"] = cfg.Provider
	envMap["VPN_TYPE"] = cfg.Protocol
	if cfg.Protocol == "wireguard" {
		if cfg.WireguardKey != "" {
			envMap["WIREGUARD_PRIVATE_KEY"] = cfg.WireguardKey
		}
		if cfg.WireguardPubKey != "" {
			envMap["WIREGUARD_PUBLIC_KEY"] = cfg.WireguardPubKey
		}
		if cfg.WireguardAddr != "" {
			envMap["WIREGUARD_ADDRESSES"] = cfg.WireguardAddr
		}
	} else if cfg.Protocol == "openvpn" {
		if cfg.OpenVPNUser != "" {
			envMap["OPENVPN_USER"] = cfg.OpenVPNUser
		}
		if cfg.OpenVPNPass != "" {
			envMap["OPENVPN_PASSWORD"] = cfg.OpenVPNPass
		}
	}
	if cfg.Country != "" {
		envMap["SERVER_COUNTRIES"] = cfg.Country
	}
	if cfg.City != "" {
		envMap["SERVER_CITIES"] = cfg.City
	}
	if cfg.RelayPort > 0 {
		envMap["FIREWALL_INPUT_PORTS"] = fmt.Sprintf("%d", cfg.RelayPort)
	}
	applyWireguardDNSToGluetunEnv(envMap)
	return envMap
}

func applyWireguardDNSToGluetunEnv(env map[string]string) {
	dns := strings.TrimSpace(env["WIREGUARD_DNS"])
	delete(env, "WIREGUARD_DNS")
	if dns == "" || strings.TrimSpace(env["DNS_UPSTREAM_PLAIN_ADDRESSES"]) != "" {
		return
	}

	addresses := make([]string, 0)
	for _, item := range strings.Split(dns, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		addresses = append(addresses, normalizePlainDNSAddress(item))
	}
	if len(addresses) == 0 {
		return
	}

	env["DNS_UPSTREAM_RESOLVER_TYPE"] = "plain"
	env["DNS_UPSTREAM_PLAIN_ADDRESSES"] = strings.Join(addresses, ",")
}

func normalizePlainDNSAddress(value string) string {
	if addrPort, err := netip.ParseAddrPort(value); err == nil {
		return addrPort.String()
	}
	if addr, err := netip.ParseAddr(value); err == nil {
		return netip.AddrPortFrom(addr, 53).String()
	}
	if strings.Count(value, ":") == 1 {
		return value
	}
	return value + ":53"
}

func orderedGluetunEnv(env map[string]string, protocol string) []string {
	order := []string{
		"VPN_SERVICE_PROVIDER",
		"VPN_TYPE",
		"SERVER_COUNTRIES",
		"SERVER_CITIES",
		"WIREGUARD_ENDPOINT_IP",
		"WIREGUARD_ENDPOINT_PORT",
		"WIREGUARD_PUBLIC_KEY",
		"WIREGUARD_PRIVATE_KEY",
		"WIREGUARD_PRESHARED_KEY",
		"WIREGUARD_ADDRESSES",
		"WIREGUARD_ALLOWED_IPS",
		"WIREGUARD_MTU",
		"WIREGUARD_PERSISTENT_KEEPALIVE_INTERVAL",
		"DNS_UPSTREAM_RESOLVER_TYPE",
		"DNS_UPSTREAM_PLAIN_ADDRESSES",
		"FIREWALL_INPUT_PORTS",
	}
	if protocol == "openvpn" {
		order = []string{
			"VPN_SERVICE_PROVIDER",
			"VPN_TYPE",
			"SERVER_COUNTRIES",
			"SERVER_CITIES",
			"OPENVPN_USER",
			"OPENVPN_PASSWORD",
			"DNS_UPSTREAM_RESOLVER_TYPE",
			"DNS_UPSTREAM_PLAIN_ADDRESSES",
			"FIREWALL_INPUT_PORTS",
		}
	}

	seen := make(map[string]bool, len(env))
	result := make([]string, 0, len(env))
	for _, key := range order {
		if value, ok := env[key]; ok {
			result = append(result, key+"="+value)
			seen[key] = true
		}
	}

	extras := make([]string, 0, len(env))
	for key := range env {
		if !seen[key] {
			extras = append(extras, key)
		}
	}
	sort.Strings(extras)
	for _, key := range extras {
		result = append(result, key+"="+env[key])
	}

	return result
}

func (c *Client) CreateRelayContainer(ctx context.Context, cfg RelayConfig) error {
	containerName := "relay-" + cfg.Name
	gluetunName := "gluetun-" + cfg.Name
	port := fmt.Sprintf("%d", cfg.ListenPort)
	imageRef := "sni-gateway-relay:latest"

	_ = c.RemoveContainer(ctx, containerName, true)
	if err := c.ensureImage(ctx, imageRef); err != nil {
		return fmt.Errorf("ensure relay image: %w", err)
	}

	deadline := time.Now().Add(containerRunningTimeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			remaining = containerRunningPollInterval
		}
		if err := c.waitForContainerRunning(ctx, gluetunName, remaining); err != nil {
			return fmt.Errorf("wait for gluetun before creating relay: %w", err)
		}

		// Relay shares gluetun's network namespace so all its outbound connections
		// are routed through the VPN tunnel. The relay image is a nginx stream proxy
		// that reads the TLS ClientHello SNI and connects to SNI_HOST:443 via VPN.
		// No host-port publishing needed - nginx (gateway) reaches it via vpn-net.
		// Build the image with: docker build -t sni-gateway-relay:latest ./relay
		resp, err := c.cli.ContainerCreate(ctx,
			&container.Config{
				Image: imageRef,
				Env: []string{
					"RELAY_PORT=" + port,
					"RELAY_RESOLVER=127.0.0.1",
					"RELAY_DNS_VALID=300s",
					"RELAY_RESOLVER_TIMEOUT=2s",
					"RELAY_CONNECT_TIMEOUT=5s",
					"SNI_RELAY_PROFILE=" + relayProfile,
				},
			},
			&container.HostConfig{
				NetworkMode:   container.NetworkMode("container:" + gluetunName),
				RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
				Resources: container.Resources{
					Ulimits: []*units.Ulimit{
						{Name: "nofile", Soft: 65535, Hard: 65535},
					},
				},
			},
			&network.NetworkingConfig{},
			nil,
			containerName,
		)
		if err != nil {
			wrappedErr := fmt.Errorf("create relay container: %w", err)
			if !isContainerNamespaceRestartingError(wrappedErr) || time.Now().After(deadline) {
				return wrappedErr
			}
			continue
		}

		if err := c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
			_ = c.RemoveContainer(ctx, containerName, true)
			wrappedErr := fmt.Errorf("start relay container: %w", err)
			if !isContainerNamespaceRestartingError(wrappedErr) || time.Now().After(deadline) {
				return wrappedErr
			}
			continue
		}

		return nil
	}
}

func (c *Client) StartContainer(ctx context.Context, name string) error {
	return c.cli.ContainerStart(ctx, name, container.StartOptions{})
}

func (c *Client) StopContainer(ctx context.Context, name string) error {
	timeout := 30
	if err := c.cli.ContainerStop(ctx, name, container.StopOptions{Timeout: &timeout}); err != nil {
		if client.IsErrNotFound(err) {
			return nil
		}
		return err
	}
	return nil
}

func (c *Client) RestartContainer(ctx context.Context, name string) error {
	timeout := 30
	return c.cli.ContainerRestart(ctx, name, container.StopOptions{Timeout: &timeout})
}

func (c *Client) RemoveContainer(ctx context.Context, name string, force bool) error {
	if err := c.cli.ContainerRemove(ctx, name, container.RemoveOptions{Force: force}); err != nil {
		if client.IsErrNotFound(err) {
			return nil
		}
		return err
	}
	return nil
}

func (c *Client) GetContainerStatus(ctx context.Context, name string) (string, error) {
	info, err := c.cli.ContainerInspect(ctx, name)
	if err != nil {
		if client.IsErrNotFound(err) {
			return "not_found", nil
		}
		return "", err
	}
	return info.State.Status, nil
}

func (c *Client) WaitForContainerRunning(ctx context.Context, name string) error {
	return c.waitForContainerRunning(ctx, name, containerRunningTimeout)
}

func (c *Client) waitForContainerRunning(ctx context.Context, name string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = containerRunningTimeout
	}

	deadline := time.Now().Add(timeout)
	lastStatus := "unknown"
	runningSince := time.Time{}
	for {
		info, err := c.cli.ContainerInspect(ctx, name)
		if err != nil {
			if client.IsErrNotFound(err) {
				return fmt.Errorf("%s container not found", name)
			}
			return fmt.Errorf("inspect %s: %w", name, err)
		}

		if info.State != nil {
			lastStatus = info.State.Status
			if info.State.Running && !info.State.Restarting {
				if runningSince.IsZero() {
					runningSince = time.Now()
				}
				if time.Since(runningSince) >= containerRunningStableDuration {
					return nil
				}
			} else {
				runningSince = time.Time{}
			}
			if info.State.Status == "dead" || info.State.Status == "exited" {
				return fmt.Errorf("%s container is %s%s", name, info.State.Status, containerStateDetail(info.State.ExitCode, info.State.Error))
			}
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("%s container did not become running within %s (last status: %s)", name, timeout, lastStatus)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(containerRunningPollInterval):
		}
	}
}

func containerStateDetail(exitCode int, stateErr string) string {
	parts := make([]string, 0, 2)
	if exitCode != 0 {
		parts = append(parts, fmt.Sprintf("exit code %d", exitCode))
	}
	if strings.TrimSpace(stateErr) != "" {
		parts = append(parts, strings.TrimSpace(stateErr))
	}
	if len(parts) == 0 {
		return ""
	}
	return ": " + strings.Join(parts, ", ")
}

func (c *Client) errorWithContainerLogs(ctx context.Context, name string, err error) error {
	logs, logsErr := c.containerLogsTail(ctx, name, 40)
	if logsErr != nil || strings.TrimSpace(logs) == "" {
		return err
	}
	return fmt.Errorf("%w; gluetun logs:\n%s", err, strings.TrimSpace(logs))
}

func (c *Client) containerLogsTail(ctx context.Context, name string, tail int) (string, error) {
	opts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       fmt.Sprintf("%d", tail),
	}
	rc, err := c.cli.ContainerLogs(ctx, name, opts)
	if err != nil {
		return "", err
	}
	defer rc.Close()

	buf := new(strings.Builder)
	errBuf := new(strings.Builder)
	if _, err = stdcopy.StdCopy(buf, errBuf, rc); err != nil {
		return "", err
	}
	output := strings.TrimSpace(buf.String())
	if strings.TrimSpace(errBuf.String()) != "" {
		if output != "" {
			output += "\n"
		}
		output += strings.TrimSpace(errBuf.String())
	}
	return output, nil
}

func isContainerNamespaceRestartingError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "cannot join network namespace of container") &&
		strings.Contains(msg, "is restarting")
}

func (c *Client) GluetunNeedsUpgrade(ctx context.Context, name string, relayPort int) (bool, error) {
	info, err := c.cli.ContainerInspect(ctx, "gluetun-"+name)
	if err != nil {
		if client.IsErrNotFound(err) {
			return false, nil
		}
		return false, err
	}

	if string(info.HostConfig.NetworkMode) != VPNNetwork {
		return true, nil
	}

	if _, ok := info.NetworkSettings.Networks[VPNNetwork]; !ok {
		return true, nil
	}

	expected := "FIREWALL_INPUT_PORTS=" + fmt.Sprintf("%d", relayPort)
	for _, env := range info.Config.Env {
		if env == expected {
			return c.relayNeedsUpgrade(ctx, name)
		}
	}

	return true, nil
}

func (c *Client) relayNeedsUpgrade(ctx context.Context, name string) (bool, error) {
	info, err := c.cli.ContainerInspect(ctx, "relay-"+name)
	if err != nil {
		if client.IsErrNotFound(err) {
			return false, nil
		}
		return false, err
	}

	expected := "SNI_RELAY_PROFILE=" + relayProfile
	for _, env := range info.Config.Env {
		if env == expected {
			return false, nil
		}
	}

	return true, nil
}

func (c *Client) GetContainerLogs(ctx context.Context, name string, tail int) (string, error) {
	return c.containerLogsTail(ctx, name, tail)
}

type ContainerInfo struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Image  string `json:"image"`
}

func (c *Client) ListContainersByPrefix(ctx context.Context, prefix string) ([]ContainerInfo, error) {
	f := filters.NewArgs()
	f.Add("name", prefix)

	containers, err := c.cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: f,
	})
	if err != nil {
		return nil, err
	}

	var result []ContainerInfo
	for _, ct := range containers {
		name := ct.ID
		if len(ct.Names) > 0 {
			name = strings.TrimPrefix(ct.Names[0], "/")
		}
		result = append(result, ContainerInfo{
			Name:   name,
			Status: ct.State,
			Image:  ct.Image,
		})
	}
	return result, nil
}

// KillContainerByComposeService sends a signal to the first container matching the
// given docker-compose project and service labels.
func (c *Client) KillContainerByComposeService(ctx context.Context, project, service, signal string) error {
	f := filters.NewArgs()
	f.Add("label", "com.docker.compose.project="+project)
	f.Add("label", "com.docker.compose.service="+service)

	containers, err := c.cli.ContainerList(ctx, container.ListOptions{Filters: f})
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}
	if len(containers) == 0 {
		return fmt.Errorf("no running container found for service %s/%s", project, service)
	}

	return c.cli.ContainerKill(ctx, containers[0].ID, signal)
}

// GetPublicIP fetches the public IP from gluetun's control server inside the container
func (c *Client) GetPublicIP(ctx context.Context, exitName string) (string, error) {
	execResp, err := c.cli.ContainerExecCreate(ctx, "gluetun-"+exitName, types.ExecConfig{
		Cmd:          []string{"wget", "-qO-", "http://localhost:8000/v1/publicip/ip"},
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return "", err
	}

	attachResp, err := c.cli.ContainerExecAttach(ctx, execResp.ID, types.ExecStartCheck{})
	if err != nil {
		return "", err
	}
	defer attachResp.Close()

	buf := new(strings.Builder)
	errBuf := new(strings.Builder)
	if _, err := stdcopy.StdCopy(buf, errBuf, attachResp.Reader); err != nil {
		return "", err
	}
	output := strings.TrimSpace(buf.String())
	if output == "" {
		output = strings.TrimSpace(errBuf.String())
	}

	var result struct {
		PublicIP string `json:"public_ip"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return output, nil
	}
	return result.PublicIP, nil
}
