package node

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/strslice"
)

const (
	serverLabel = "control-plane.server-id"
	configLabel = "control-plane.config"
)

func (a *Agent) createServer(ctx context.Context, message Message) (string, error) {
	payload, ok := message.Payload.(map[string]any)
	if !ok || message.ServerID == "" {
		return "", fmt.Errorf("server configuration is missing")
	}
	imageName := strings.TrimSpace(textValue(payload["image"]))
	if imageName == "" {
		return "", fmt.Errorf("Config has no runtime Docker image")
	}
	dataDirectory := textValue(payload["dataDirectory"])
	if dataDirectory == "" {
		dataDirectory = "/home/container"
	}
	if !strings.HasPrefix(dataDirectory, "/") || dataDirectory == "/" {
		return "", fmt.Errorf("Config data directory must be an absolute non-root path")
	}

	labels := map[string]string{serverLabel: message.ServerID}
	if err := a.removeServerContainers(ctx, message.ServerID); err != nil {
		return "", err
	}
	diskSize := int64(number(payload["diskSizeBytes"], 0))
	persistentMount, err := a.ensureDataMount(ctx, message.ServerID, diskSize, dataDirectory)
	if err != nil {
		return "", fmt.Errorf("prepare server data disk: %w", err)
	}
	if err := a.pullImageIfMissing(ctx, imageName); err != nil {
		return "", err
	}
	if install, ok := payload["install"].(map[string]any); ok && strings.TrimSpace(textValue(install["script"])) != "" {
		if err := a.InstallServer(ctx, message.ServerID, persistentMount, labels, payload, install); err != nil {
			return "", err
		}
	}

	ports, bindings, err := dockerPorts(payload["ports"])
	if err != nil {
		return "", err
	}
	if err := OpenAllocatedPorts(ctx, bindings); err != nil {
		return "", err
	}
	labels[configLabel] = configHash(payload)
	config := &container.Config{
		Image: imageName, Env: stringSlice(payload["environment"]),
		Cmd:        strslice.StrSlice(stringSlice(payload["command"])),
		Entrypoint: strslice.StrSlice(stringSlice(payload["entrypoint"])),
		WorkingDir: textValue(payload["workingDir"]), User: textValue(payload["user"]),
		ExposedPorts: ports, Labels: labels, Tty: true, OpenStdin: true,
	}
	resources := container.Resources{}
	if cpu := number(payload["cpuLimit"], 0); cpu > 0 {
		resources.NanoCPUs = int64(cpu * 1e9)
	}
	if memory := number(payload["memoryMB"], 0); memory > 0 {
		resources.Memory = int64(memory * 1024 * 1024)
	}
	mounts := []mount.Mount{persistentMount}
	volumeIDs := map[string]bool{message.ServerID: true}
	for _, raw := range anySlice(payload["volumes"]) {
		volume, ok := raw.(map[string]any)
		if !ok {
			return "", fmt.Errorf("invalid attached volume")
		}
		volumeID := textValue(volume["id"])
		mountPath := strings.TrimSpace(textValue(volume["mountPath"]))
		if volumeIDs[volumeID] || !validNodeVolumeMountPath(mountPath, dataDirectory) {
			return "", fmt.Errorf("invalid or duplicate volume mount path")
		}
		volumeIDs[volumeID] = true
		sizeBytes := int64(number(volume["sizeBytes"], 0))
		volumeMount, err := a.ensureDataMount(ctx, volumeID, sizeBytes, mountPath)
		if err != nil {
			return "", fmt.Errorf("prepare volume %s: %w", textValue(volume["name"]), err)
		}
		mounts = append(mounts, volumeMount)
	}
	hostConfig := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{Name: "no"},
		PortBindings:  bindings, Resources: resources,
		Mounts: mounts,
	}
	created, err := a.docker.ContainerCreate(ctx, config, hostConfig, &network.NetworkingConfig{}, nil, "control-plane-"+message.ServerID)
	if err != nil {
		return "", fmt.Errorf("create server container: %w", err)
	}
	if err := a.docker.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		_ = a.docker.ContainerRemove(ctx, created.ID, container.RemoveOptions{Force: true})
		return "", fmt.Errorf("start server container: %w", err)
	}
	time.Sleep(250 * time.Millisecond)
	if inspect, err := a.docker.ContainerInspect(ctx, created.ID); err == nil && !inspect.State.Running {
		logs, _ := a.installerLogs(ctx, created.ID)
		_ = a.docker.ContainerRemove(ctx, created.ID, container.RemoveOptions{Force: true})
		if len(logs) > 12000 {
			logs = logs[len(logs)-12000:]
		}
		return "", fmt.Errorf("server process exited during startup (code %d): %s", inspect.State.ExitCode, strings.TrimSpace(logs))
	}
	return created.ID, nil
}

func anySlice(value any) []any {
	items, _ := value.([]any)
	return items
}

func validNodeVolumeMountPath(value, dataDirectory string) bool {
	if value == "" || !strings.HasPrefix(value, "/") || value == "/" || path.Clean(value) != value || value == dataDirectory {
		return false
	}
	return strings.HasPrefix(value, "/mnt/") || strings.HasPrefix(value, strings.TrimSuffix(dataDirectory, "/")+"/")
}

func (a *Agent) removeServerContainers(ctx context.Context, serverID string) error {
	labelFilters := filters.NewArgs(filters.Arg("label", serverLabel+"="+serverID))
	items, err := a.docker.ContainerList(ctx, container.ListOptions{All: true, Filters: labelFilters})
	if err != nil {
		return err
	}
	for _, item := range items {
		if err := a.docker.ContainerRemove(ctx, item.ID, container.RemoveOptions{Force: true}); err != nil && !strings.Contains(strings.ToLower(err.Error()), "no such container") {
			return fmt.Errorf("remove previous server container: %w", err)
		}
	}
	return nil
}

func (a *Agent) pullImage(ctx context.Context, name string) error {
	stream, err := a.docker.ImagePull(ctx, name, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pull Docker image %s: %w", name, err)
	}
	defer stream.Close()
	decoder := json.NewDecoder(bufio.NewReader(stream))
	for {
		var message struct {
			Error       string `json:"error"`
			ErrorDetail struct {
				Message string `json:"message"`
			} `json:"errorDetail"`
		}
		if err := decoder.Decode(&message); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read Docker image pull for %s: %w", name, err)
		}
		if message.ErrorDetail.Message != "" {
			return fmt.Errorf("pull Docker image %s: %s", name, message.ErrorDetail.Message)
		}
		if message.Error != "" {
			return fmt.Errorf("pull Docker image %s: %s", name, message.Error)
		}
	}
}

func (a *Agent) pullImageIfMissing(ctx context.Context, name string) error {
	if _, err := a.docker.ImageInspect(ctx, name); err == nil {
		return nil
	}
	return a.pullImage(ctx, name)
}

func configHash(payload map[string]any) string {
	data, _ := json.Marshal(payload)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func textValue(value any) string {
	text, _ := value.(string)
	return text
}
