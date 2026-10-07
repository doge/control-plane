package node

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/docker/docker/api/types/mount"
	volumetypes "github.com/docker/docker/api/types/volume"
)

const dockerVolumeIDLabel = "control-plane.volume-id"
const minimumNodeVolumeBytes int64 = 128 * 1024 * 1024

func storageMode() string {
	if runtime.GOOS == "darwin" {
		return "docker-volume"
	}
	return "ext4"
}

func dockerVolumeName(id string) (string, error) {
	if _, err := virtualVolumeRoot(id); err != nil {
		return "", err
	}
	return "control-plane-volume-" + id, nil
}

func (a *Agent) ensureDockerVolume(ctx context.Context, id string) (string, error) {
	name, err := dockerVolumeName(id)
	if err != nil {
		return "", err
	}
	_, err = a.docker.VolumeCreate(ctx, volumetypes.CreateOptions{
		Name: name,
		Labels: map[string]string{
			dockerVolumeIDLabel: id,
		},
	})
	if err != nil {
		return "", fmt.Errorf("create Docker volume: %w", err)
	}
	return name, nil
}

func (a *Agent) ensureDataMount(ctx context.Context, id string, sizeBytes int64, target string) (mount.Mount, error) {
	if sizeBytes < minimumNodeVolumeBytes {
		return mount.Mount{}, fmt.Errorf("volume size must be at least 128 MB")
	}
	if runtime.GOOS == "darwin" {
		name, err := a.ensureDockerVolume(ctx, id)
		if err != nil {
			return mount.Mount{}, err
		}
		return mount.Mount{Type: mount.TypeVolume, Source: name, Target: target}, nil
	}
	dataPath, err := ensureVirtualVolume(ctx, id, sizeBytes)
	if err != nil {
		return mount.Mount{}, err
	}
	return mount.Mount{Type: mount.TypeBind, Source: dataPath, Target: target}, nil
}

func (a *Agent) removeServerVolume(ctx context.Context, id string) error {
	if runtime.GOOS != "darwin" {
		return removeVirtualVolume(ctx, id)
	}
	name, err := dockerVolumeName(id)
	if err != nil {
		return err
	}
	if err := a.docker.VolumeRemove(ctx, name, true); err != nil && !strings.Contains(strings.ToLower(err.Error()), "no such volume") {
		return fmt.Errorf("remove Docker volume: %w", err)
	}
	return nil
}

func (a *Agent) prepareServerVolume(ctx context.Context, operation, id string, sizeBytes int64) error {
	if sizeBytes < minimumNodeVolumeBytes {
		return fmt.Errorf("volume size must be at least 128 MB")
	}
	if runtime.GOOS != "darwin" {
		_, err := ensureVirtualVolume(ctx, id, sizeBytes)
		return err
	}
	if operation == "server_volume_create" {
		_, err := a.ensureDockerVolume(ctx, id)
		return err
	}
	name, err := dockerVolumeName(id)
	if err != nil {
		return err
	}
	if _, err := a.docker.VolumeInspect(ctx, name); err != nil {
		return fmt.Errorf("inspect Docker volume: %w", err)
	}
	return nil
}
