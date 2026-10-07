package node

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
)

func (a *Agent) inspectServer(ctx context.Context, m Message) (map[string]any, error) {
	if m.ContainerID == "" {
		return nil, fmt.Errorf("server has no container id; deploy it first")
	}
	inspect, err := a.docker.ContainerInspect(ctx, m.ContainerID)
	if err != nil {
		return nil, err
	}
	result := map[string]any{"image": inspect.Config.Image, "entrypoint": inspect.Config.Entrypoint, "command": inspect.Config.Cmd, "running": inspect.State.Running, "cpuPercent": 0.0, "memoryUsedBytes": uint64(0)}
	if !inspect.State.Running {
		return result, nil
	}
	statsCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	stats, err := a.docker.ContainerStats(statsCtx, m.ContainerID, true)
	if err == nil {
		defer stats.Body.Close()
		var sample container.StatsResponse
		decoder := json.NewDecoder(stats.Body)
		if decoder.Decode(&sample) == nil {
			var next container.StatsResponse
			if decoder.Decode(&next) == nil {
				sample = next
			}
			cpuDelta, systemDelta := float64(0), float64(0)
			if sample.CPUStats.CPUUsage.TotalUsage > sample.PreCPUStats.CPUUsage.TotalUsage {
				cpuDelta = float64(sample.CPUStats.CPUUsage.TotalUsage - sample.PreCPUStats.CPUUsage.TotalUsage)
			}
			if sample.CPUStats.SystemUsage > sample.PreCPUStats.SystemUsage {
				systemDelta = float64(sample.CPUStats.SystemUsage - sample.PreCPUStats.SystemUsage)
			}
			cpus := sample.CPUStats.OnlineCPUs
			if cpus == 0 {
				cpus = uint32(len(sample.CPUStats.CPUUsage.PercpuUsage))
			}
			if cpuDelta > 0 && systemDelta > 0 && cpus > 0 {
				result["cpuPercent"] = cpuDelta / systemDelta * float64(cpus) * 100
			}
			used := sample.MemoryStats.Usage
			if inactive := sample.MemoryStats.Stats["inactive_file"]; used > inactive {
				used -= inactive
			}
			result["memoryUsedBytes"] = used
			result["memoryLimitBytes"] = sample.MemoryStats.Limit
		}
	}
	if processes, err := a.docker.ContainerTop(ctx, m.ContainerID, []string{"-eo", "pid,ppid,args"}); err == nil {
		for _, process := range processes.Processes {
			if len(process) < 3 {
				continue
			}
			command := strings.Join(process[2:], " ")
			lower := strings.ToLower(command)
			if strings.Contains(command, "java ") && !strings.Contains(lower, "mc-server-runner") && !strings.Contains(lower, "mcimagehelper") && !strings.Contains(lower, "mc-image-helper") {
				result["gameCommand"] = command
				break
			}
		}
	}
	return result, nil
}

func (a *Agent) deleteBackup(ctx context.Context, m Message, payload map[string]any) (map[string]any, error) {
	backupID := textValue(payload["backupId"])
	if backupID == "" || filepath.Base(backupID) != backupID || strings.Contains(backupID, "..") {
		return nil, fmt.Errorf("invalid backup id")
	}
	imageName := "control-plane-backup-" + m.ServerID + ":" + backupID
	if _, err := a.docker.ImageRemove(ctx, imageName, image.RemoveOptions{Force: true, PruneChildren: true}); err != nil && !strings.Contains(strings.ToLower(err.Error()), "no such image") {
		return nil, err
	}
	if err := os.RemoveAll(filepath.Join(backupRoot(), m.ServerID, backupID)); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func (a *Agent) createBackup(ctx context.Context, m Message, payload map[string]any) (map[string]any, error) {
	backupID := textValue(payload["backupId"])
	if backupID == "" || m.ContainerID == "" {
		return nil, fmt.Errorf("backup requires a deployed server")
	}
	reference := "control-plane-backup-" + m.ServerID + ":" + backupID
	commit, err := a.docker.ContainerCommit(ctx, m.ContainerID, container.CommitOptions{Reference: reference, Comment: "Control Plane backup " + backupID})
	if err != nil {
		return nil, err
	}
	size, err := a.saveMountedVolumes(ctx, m.ContainerID, m.ServerID, backupID)
	if err != nil {
		return nil, err
	}
	if inspect, err := a.docker.ImageInspect(ctx, commit.ID); err == nil {
		size += int64(inspect.Size)
	}
	return map[string]any{"id": backupID, "image": reference, "imageId": commit.ID, "sizeBytes": size}, nil
}

func (a *Agent) restoreServer(ctx context.Context, m Message, payload map[string]any) (map[string]any, error) {
	backupImage := textValue(payload["image"])
	config, _ := payload["config"].(map[string]any)
	if backupImage == "" || config == nil {
		return nil, fmt.Errorf("backup data missing")
	}
	config["image"], config["install"] = backupImage, nil
	backupID := textValue(payload["backupId"])
	id, err := a.createServer(ctx, Message{ServerID: m.ServerID, Payload: config})
	if err != nil {
		return nil, err
	}
	if err := a.restoreMountedVolumes(ctx, id, m.ServerID, backupID); err != nil {
		return nil, err
	}
	return map[string]any{"containerId": id, "status": "running"}, nil
}

func (a *Agent) deleteServer(ctx context.Context, m Message, payload map[string]any) (map[string]any, error) {
	labelFilters := filters.NewArgs(filters.Arg("label", serverLabel+"="+m.ServerID))
	containers, err := a.docker.ContainerList(ctx, container.ListOptions{All: true, Filters: labelFilters})
	if err != nil {
		return nil, err
	}
	imageIDs := make(map[string]bool, len(containers))
	for _, item := range containers {
		if item.ImageID != "" {
			imageIDs[item.ImageID] = true
		}
		if err := a.docker.ContainerRemove(ctx, item.ID, container.RemoveOptions{Force: true, RemoveVolumes: true}); err != nil && !strings.Contains(strings.ToLower(err.Error()), "no such container") {
			return nil, err
		}
	}
	for _, volumeID := range append([]string{m.ServerID}, stringSlice(payload["volumeIds"])...) {
		if err := a.removeServerVolume(ctx, volumeID); err != nil {
			return nil, err
		}
	}
	backupImagePrefix := "control-plane-backup-" + m.ServerID + ":"
	backupImages := make(map[string]bool)
	for _, raw := range stringSlice(payload["backupImages"]) {
		if strings.HasPrefix(raw, backupImagePrefix) {
			backupImages[raw] = true
		}
	}
	if images, err := a.docker.ImageList(ctx, image.ListOptions{All: true}); err == nil {
		for _, item := range images {
			for _, tag := range item.RepoTags {
				if strings.HasPrefix(tag, backupImagePrefix) {
					backupImages[tag] = true
				}
			}
		}
	} else {
		return nil, err
	}
	for backupImage := range backupImages {
		if _, err := a.docker.ImageRemove(ctx, backupImage, image.RemoveOptions{Force: true, PruneChildren: true}); err != nil && !strings.Contains(strings.ToLower(err.Error()), "no such image") {
			return nil, err
		}
	}
	if err := os.RemoveAll(filepath.Join(backupRoot(), m.ServerID)); err != nil {
		return nil, err
	}
	if prune, _ := payload["pruneUnusedImages"].(bool); prune && len(imageIDs) > 0 {
		remaining, err := a.docker.ContainerList(ctx, container.ListOptions{All: true})
		if err != nil {
			return nil, err
		}
		inUse := make(map[string]bool, len(remaining))
		for _, item := range remaining {
			inUse[item.ImageID] = true
		}
		for imageID := range imageIDs {
			if inUse[imageID] {
				continue
			}
			if _, err := a.docker.ImageRemove(ctx, imageID, image.RemoveOptions{}); err != nil && !strings.Contains(strings.ToLower(err.Error()), "no such image") {
				return nil, err
			}
		}
	}
	return map[string]any{"ok": true}, nil
}

func (a *Agent) serverAction(ctx context.Context, m Message, payload map[string]any) (map[string]any, error) {
	action := textValue(payload["action"])
	if action == "deploy" {
		id, err := a.createServer(ctx, m)
		return map[string]any{"containerId": id, "status": "running"}, err
	}
	if m.ContainerID == "" {
		return nil, fmt.Errorf("server has no container id; deploy it first")
	}
	inspect, err := a.docker.ContainerInspect(ctx, m.ContainerID)
	if err != nil {
		return nil, err
	}
	switch action {
	case "start":
		if inspect.State.Paused {
			err = a.docker.ContainerUnpause(ctx, m.ContainerID)
		} else if inspect.State.Running {
			return map[string]any{"status": "running"}, nil
		} else {
			a.detachConsole(m.ServerID)
			err = a.docker.ContainerStart(ctx, m.ContainerID, container.StartOptions{})
			if err != nil {
				if latest, latestErr := a.docker.ContainerInspect(ctx, m.ContainerID); latestErr == nil && latest.State.Running {
					err = nil
				}
			}
		}
	case "pause":
		err = a.docker.ContainerPause(ctx, m.ContainerID)
	case "resume":
		err = a.docker.ContainerUnpause(ctx, m.ContainerID)
	case "kill":
		if !inspect.State.Running {
			return map[string]any{"status": "stopped"}, nil
		}
		if inspect.State.Paused {
			if err := a.docker.ContainerUnpause(ctx, m.ContainerID); err != nil {
				return nil, err
			}
		}
		a.detachConsole(m.ServerID)
		zero := 0
		err = a.docker.ContainerStop(ctx, m.ContainerID, container.StopOptions{Timeout: &zero})
	default:
		return nil, fmt.Errorf("unknown server action")
	}
	if err != nil {
		return nil, err
	}
	status := map[string]string{"start": "running", "pause": "paused", "resume": "running", "kill": "stopped"}[action]
	return map[string]any{"status": status}, nil
}
