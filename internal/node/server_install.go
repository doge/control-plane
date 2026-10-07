package node

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/strslice"
)

const installLabel = "control-plane.install"

// InstallServer creates and runs the installer container for a server.
func (a *Agent) InstallServer(ctx context.Context, serverID string, dataMount mount.Mount, labels map[string]string, server, install map[string]any) error {
	imageName := strings.TrimSpace(textValue(install["image"]))
	script := strings.TrimSpace(textValue(install["script"]))
	if imageName == "" || script == "" {
		return fmt.Errorf("Config install image and script are required")
	}
	if err := a.pullImageIfMissing(ctx, imageName); err != nil {
		return fmt.Errorf("pull installer image: %w", err)
	}
	entrypoint := textValue(install["entrypoint"])
	if entrypoint == "" {
		entrypoint = "/bin/sh"
	}
	installLabels := make(map[string]string, len(labels)+1)
	for key, value := range labels {
		installLabels[key] = value
	}
	installLabels[installLabel] = "true"
	config := &container.Config{
		Image: imageName, Env: stringSlice(server["environment"]),
		Entrypoint: strslice.StrSlice{entrypoint}, Cmd: strslice.StrSlice{"-lc", script},
		WorkingDir: "/mnt/server", Labels: installLabels, Tty: true,
	}
	dataMount.Target = "/mnt/server"
	hostConfig := &container.HostConfig{Mounts: []mount.Mount{dataMount}}
	created, err := a.docker.ContainerCreate(ctx, config, hostConfig, &network.NetworkingConfig{}, nil, "control-plane-install-"+serverID)
	if err != nil {
		return fmt.Errorf("create Config installer: %w", err)
	}
	defer func() {
		_ = a.docker.ContainerRemove(context.Background(), created.ID, container.RemoveOptions{Force: true})
	}()
	if err := a.docker.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("start Config installer: %w", err)
	}
	wait, waitErr := a.docker.ContainerWait(ctx, created.ID, container.WaitConditionNotRunning)
	for {
		select {
		case result, ok := <-wait:
			if !ok {
				return fmt.Errorf("wait for Config installer: Docker closed the wait response without an exit status")
			}
			logs, logErr := a.installerLogs(ctx, created.ID)
			if logErr != nil {
				logs += "\nCould not read installer output: " + logErr.Error()
			}
			if result.StatusCode != 0 {
				if len(logs) > 12000 {
					logs = logs[len(logs)-12000:]
				}
				return fmt.Errorf("Config installer exited with status %d: %s", result.StatusCode, strings.TrimSpace(logs))
			}
			return nil
		case err, ok := <-waitErr:
			if !ok || err == nil {
				waitErr = nil
				continue
			}
			return fmt.Errorf("wait for Config installer: %w", err)
		case <-ctx.Done():
			return fmt.Errorf("Config installer timed out: %w", ctx.Err())
		}
	}
}

func (a *Agent) installerLogs(ctx context.Context, containerID string) (string, error) {
	stream, err := a.docker.ContainerLogs(ctx, containerID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return "", err
	}
	defer stream.Close()
	data, err := io.ReadAll(io.LimitReader(stream, 1<<20))
	return string(data), err
}
