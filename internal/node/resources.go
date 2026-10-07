package node

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
)

func (a *Agent) managedServerContainers(ctx context.Context) ([]container.Summary, error) {
	filter := filters.NewArgs(filters.Arg("label", serverLabel))
	items, err := a.docker.ContainerList(ctx, container.ListOptions{All: true, Filters: filter})
	if err != nil {
		return nil, err
	}
	managed := make([]container.Summary, 0, len(items))
	for _, item := range items {
		if item.Labels[installLabel] == "true" || item.Labels[serverLabel] == "" {
			continue
		}
		managed = append(managed, item)
	}
	return managed, nil
}

func (a *Agent) serverStatuses(ctx context.Context) ([]map[string]any, error) {
	items, err := a.managedServerContainers(ctx)
	if err != nil {
		return nil, err
	}
	statuses := make([]map[string]any, 0, len(items))
	for _, item := range items {
		status := "stopped"
		if item.State == "running" || item.State == "restarting" {
			status = "running"
		} else if item.State == "paused" {
			status = "paused"
		}
		statuses = append(statuses, map[string]any{
			"serverId": item.Labels[serverLabel], "containerId": item.ID, "status": status,
		})
	}
	return statuses, nil
}

func StopManagedServers() error {
	docker, err := newDockerClient()
	if err != nil {
		return err
	}
	defer docker.Close()
	return (&Agent{docker: docker}).stopManagedServers(context.Background())
}

func DisableManagedServerRestarts() error {
	docker, err := newDockerClient()
	if err != nil {
		return err
	}
	defer docker.Close()
	items, err := (&Agent{docker: docker}).managedServerContainers(context.Background())
	if err != nil {
		return err
	}
	var failures []error
	for _, item := range items {
		inspect, err := docker.ContainerInspect(context.Background(), item.ID)
		if err != nil {
			failures = append(failures, fmt.Errorf("inspect %s: %w", item.ID, err))
			continue
		}
		if inspect.HostConfig == nil || inspect.HostConfig.RestartPolicy.IsNone() {
			continue
		}
		_, err = docker.ContainerUpdate(context.Background(), item.ID, container.UpdateConfig{
			Resources:     inspect.HostConfig.Resources,
			RestartPolicy: container.RestartPolicy{Name: "no"},
		})
		if err != nil {
			failures = append(failures, fmt.Errorf("disable restart policy for %s: %w", item.ID, err))
		}
	}
	return errors.Join(failures...)
}

func (a *Agent) stopManagedServers(ctx context.Context) error {
	items, err := a.managedServerContainers(ctx)
	if err != nil {
		return err
	}
	timeout := 10
	var failures []error
	for _, item := range items {
		if item.State != "running" && item.State != "paused" && item.State != "restarting" {
			continue
		}
		if item.State == "paused" {
			if err := a.docker.ContainerUnpause(ctx, item.ID); err != nil {
				failures = append(failures, fmt.Errorf("unpause server %s before stopping: %w", item.Labels[serverLabel], err))
				continue
			}
		}
		if err := a.docker.ContainerStop(ctx, item.ID, container.StopOptions{Timeout: &timeout}); err != nil {
			failures = append(failures, fmt.Errorf("stop server %s: %w", item.Labels[serverLabel], err))
		}
	}
	return errors.Join(failures...)
}

func (a *Agent) dockerResources(ctx context.Context) (map[string]any, error) {
	containers, err := a.docker.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, err
	}
	images, err := a.docker.ImageList(ctx, image.ListOptions{All: true})
	if err != nil {
		return nil, err
	}

	containerItems := make([]map[string]any, 0, len(containers))
	for _, item := range containers {
		serverID := item.Labels[serverLabel]
		lastOnlineAt := ""
		if item.State != "running" {
			if inspect, inspectErr := a.docker.ContainerInspect(ctx, item.ID); inspectErr == nil && inspect.State != nil {
				lastOnlineAt = inspect.State.FinishedAt
				if strings.HasPrefix(lastOnlineAt, "0001-01-01") {
					lastOnlineAt = ""
				}
			}
		}
		containerItems = append(containerItems, map[string]any{
			"id": item.ID, "names": item.Names, "image": item.Image,
			"imageId": item.ImageID, "state": item.State, "status": item.Status,
			"created": item.Created, "serverId": serverID, "lastOnlineAt": lastOnlineAt,
		})
	}
	imageItems := make([]map[string]any, 0, len(images))
	for _, item := range images {
		imageItems = append(imageItems, map[string]any{
			"id": item.ID, "tags": item.RepoTags, "sizeBytes": item.Size,
			"created": item.Created,
		})
	}
	return map[string]any{"containers": containerItems, "images": imageItems}, nil
}

func (a *Agent) deleteNodeContainer(ctx context.Context, payload map[string]any) (map[string]any, error) {
	id := strings.TrimSpace(textValue(payload["containerId"]))
	if id == "" {
		return nil, fmt.Errorf("container ID is required")
	}
	if err := a.docker.ContainerRemove(ctx, id, container.RemoveOptions{Force: true}); err != nil {
		return nil, fmt.Errorf("remove container: %w", err)
	}
	return map[string]any{"ok": true}, nil
}

func (a *Agent) deleteNodeImage(ctx context.Context, payload map[string]any) (map[string]any, error) {
	id := strings.TrimSpace(textValue(payload["imageId"]))
	if id == "" {
		return nil, fmt.Errorf("image ID is required")
	}
	if _, err := a.docker.ImageRemove(ctx, id, image.RemoveOptions{}); err != nil {
		return nil, fmt.Errorf("remove image: %w", err)
	}
	return map[string]any{"ok": true}, nil
}
