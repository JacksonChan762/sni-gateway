package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
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
}

type RelayConfig struct {
	Name       string
	ListenPort int
}

func (c *Client) CreateGluetunContainer(ctx context.Context, cfg GluetunConfig) error {
	containerName := "gluetun-" + cfg.Name
	imageRef := "qmcgaw/gluetun:latest"

	env := []string{
		"VPN_SERVICE_PROVIDER=" + cfg.Provider,
		"VPN_TYPE=" + cfg.Protocol,
	}
	if cfg.Protocol == "wireguard" {
		env = append(env, "WIREGUARD_PRIVATE_KEY="+cfg.WireguardKey)
		if cfg.WireguardPubKey != "" {
			env = append(env, "WIREGUARD_PUBLIC_KEY="+cfg.WireguardPubKey)
		}
		if cfg.WireguardAddr != "" {
			env = append(env, "WIREGUARD_ADDRESSES="+cfg.WireguardAddr)
		}
	} else if cfg.Protocol == "openvpn" {
		env = append(env,
			"OPENVPN_USER="+cfg.OpenVPNUser,
			"OPENVPN_PASSWORD="+cfg.OpenVPNPass,
		)
	}
	if cfg.Country != "" {
		env = append(env, "SERVER_COUNTRIES="+cfg.Country)
	}
	if cfg.City != "" {
		env = append(env, "SERVER_CITIES="+cfg.City)
	}
	if cfg.RelayPort > 0 {
		env = append(env, "FIREWALL_INPUT_PORTS="+fmt.Sprintf("%d", cfg.RelayPort))
	}

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
			CapAdd:        []string{"NET_ADMIN"},
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

	return nil
}

func (c *Client) CreateRelayContainer(ctx context.Context, cfg RelayConfig) error {
	containerName := "relay-" + cfg.Name
	port := fmt.Sprintf("%d", cfg.ListenPort)
	imageRef := "sni-gateway-relay:latest"

	_ = c.RemoveContainer(ctx, containerName, true)
	if err := c.ensureImage(ctx, imageRef); err != nil {
		return fmt.Errorf("ensure relay image: %w", err)
	}

	// Relay shares gluetun's network namespace so all its outbound connections
	// are routed through the VPN tunnel. The relay image is a nginx stream proxy
	// that reads the TLS ClientHello SNI and connects to SNI_HOST:443 via VPN.
	// No host-port publishing needed — nginx (gateway) reaches it via vpn-net.
	// Build the image with: docker build -t sni-gateway-relay:latest ./relay
	resp, err := c.cli.ContainerCreate(ctx,
		&container.Config{
			Image: imageRef,
			Env:   []string{"RELAY_PORT=" + port},
		},
		&container.HostConfig{
			NetworkMode:   container.NetworkMode("container:gluetun-" + cfg.Name),
			RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
		},
		&network.NetworkingConfig{},
		nil,
		containerName,
	)
	if err != nil {
		return fmt.Errorf("create relay container: %w", err)
	}

	return c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{})
}

func (c *Client) StartContainer(ctx context.Context, name string) error {
	return c.cli.ContainerStart(ctx, name, container.StartOptions{})
}

func (c *Client) StopContainer(ctx context.Context, name string) error {
	timeout := 30
	return c.cli.ContainerStop(ctx, name, container.StopOptions{Timeout: &timeout})
}

func (c *Client) RestartContainer(ctx context.Context, name string) error {
	timeout := 30
	return c.cli.ContainerRestart(ctx, name, container.StopOptions{Timeout: &timeout})
}

func (c *Client) RemoveContainer(ctx context.Context, name string, force bool) error {
	return c.cli.ContainerRemove(ctx, name, container.RemoveOptions{Force: force})
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
			return false, nil
		}
	}

	return true, nil
}

func (c *Client) GetContainerLogs(ctx context.Context, name string, tail int) (string, error) {
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
	if errBuf.Len() > 0 && buf.Len() == 0 {
		return errBuf.String(), nil
	}
	return buf.String(), nil
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
