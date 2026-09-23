package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
)

const SingBoxImage = "ghcr.io/sagernet/sing-box:v1.14.1"

// CreateSingBoxContainer starts a per-exit VPN and SOCKS listener on vpn-net.
func (c *Client) CreateSingBoxContainer(ctx context.Context, name string, config []byte, networkMode ...string) error {
	containerName := "singbox-" + name
	mode := VPNNetwork
	if len(networkMode) != 0 {
		mode = networkMode[0]
	}
	if err := c.ensureImage(ctx, SingBoxImage); err != nil {
		return fmt.Errorf("sing-box image: %w", err)
	}
	if err := c.RemoveContainer(ctx, containerName, true); err != nil {
		return err
	}
	resp, err := c.cli.ContainerCreate(ctx,
		&container.Config{Image: SingBoxImage, Cmd: []string{"run", "-c", "/tmp/sing-box.json"}},
		&container.HostConfig{NetworkMode: container.NetworkMode(mode), RestartPolicy: container.RestartPolicy{Name: "unless-stopped"}},
		&network.NetworkingConfig{}, nil, containerName)
	if err != nil {
		return err
	}
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	if err := writer.WriteHeader(&tar.Header{Name: "sing-box.json", Mode: 0600, Size: int64(len(config)), ModTime: time.Now()}); err != nil {
		return err
	}
	if _, err := writer.Write(config); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	if err := c.cli.CopyToContainer(ctx, resp.ID, "/tmp", &archive, types.CopyToContainerOptions{}); err != nil {
		_ = c.RemoveContainer(ctx, containerName, true)
		return fmt.Errorf("install sing-box configuration: %w", err)
	}
	if err := c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		_ = c.RemoveContainer(ctx, containerName, true)
		return err
	}
	if err := c.WaitForContainerRunning(ctx, containerName); err != nil {
		failure := c.errorWithContainerLogs(ctx, containerName, err)
		_ = c.RemoveContainer(ctx, containerName, true)
		return failure
	}
	return nil
}
