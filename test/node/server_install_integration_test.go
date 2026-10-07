package node_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/api/types/volume"
	"github.com/example/control-plane/internal/node"
)

func TestInstallServerWritesToPersistentVolume(t *testing.T) {
	if os.Getenv("CONTROL_PLANE_DOCKER_TEST") != "1" {
		t.Skip("set CONTROL_PLANE_DOCKER_TEST=1 to run the local Docker integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	agent, err := node.NewAgent(node.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer agent.Close()
	docker := agent.DockerClient()
	image := "eclipse-temurin:21-jre"
	if _, err := docker.ImageInspect(ctx, image); err != nil {
		t.Fatalf("integration test requires cached image %s: %v", image, err)
	}

	serverID := fmt.Sprintf("test-%d", time.Now().UnixNano())
	volumeName := "control-plane-install-test-" + serverID
	if _, err := docker.VolumeCreate(ctx, volume.CreateOptions{Name: volumeName}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = docker.VolumeRemove(context.Background(), volumeName, true) })

	dataMount := mount.Mount{Type: mount.TypeVolume, Source: volumeName}
	install := map[string]any{
		"image":  image,
		"script": "set -eu\nprintf 'config-ready' > /mnt/server/marker.txt",
	}
	if err := agent.InstallServer(ctx, serverID, dataMount, map[string]string{}, map[string]any{}, install); err != nil {
		t.Fatal(err)
	}

	verify, err := docker.ContainerCreate(ctx, &container.Config{
		Image: image, Entrypoint: strslice.StrSlice{"/bin/sh"},
		Cmd: strslice.StrSlice{"-lc", "test \"$(cat /mnt/server/marker.txt)\" = config-ready"},
	}, &container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeVolume, Source: volumeName, Target: "/mnt/server"}}}, nil, nil, "control-plane-verify-"+serverID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = docker.ContainerRemove(context.Background(), verify.ID, container.RemoveOptions{Force: true})
	})
	if err := docker.ContainerStart(ctx, verify.ID, container.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	wait, waitErr := docker.ContainerWait(ctx, verify.ID, container.WaitConditionNotRunning)
	for {
		select {
		case result, ok := <-wait:
			if !ok {
				t.Fatal("Docker closed the verification wait without an exit status")
			}
			if result.StatusCode != 0 {
				t.Fatalf("installer did not leave its output on the persistent volume (exit %d)", result.StatusCode)
			}
			return
		case err, ok := <-waitErr:
			if !ok || err == nil {
				waitErr = nil
				continue
			}
			t.Fatal(err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
}
