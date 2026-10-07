package node

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

const virtualVolumeOverhead int64 = 128 * 1024 * 1024

func virtualVolumeRoot(id string) (string, error) {
	if len(id) != 24 {
		return "", fmt.Errorf("invalid volume ID")
	}
	for _, c := range id {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return "", fmt.Errorf("invalid volume ID")
		}
	}
	return filepath.Join("/var/lib/control-plane-node/volumes", id), nil
}

func virtualVolumeDataPath(id string) (string, error) {
	root, err := virtualVolumeRoot(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "data"), nil
}

func runSystemCommand(ctx context.Context, name string, args ...string) error {
	output, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message != "" {
			return fmt.Errorf("%s: %w: %s", name, err, message)
		}
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func ensureVirtualVolume(ctx context.Context, id string, requestedBytes int64) (string, error) {
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("size-limited volumes require a Linux node")
	}
	if requestedBytes < minimumNodeVolumeBytes {
		return "", fmt.Errorf("volume size must be at least 128 MB")
	}
	root, err := virtualVolumeRoot(id)
	if err != nil {
		return "", err
	}
	dataPath := filepath.Join(root, "data")
	imagePath := filepath.Join(root, "disk.img")
	if err := os.MkdirAll(root, 0750); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dataPath, 0750); err != nil {
		return "", err
	}
	stat, err := os.Stat(imagePath)
	if os.IsNotExist(err) {
		if err := checkHostSpace(requestedBytes + virtualVolumeOverhead); err != nil {
			return "", err
		}
		if err := runSystemCommand(ctx, "fallocate", "-l", fmt.Sprint(requestedBytes+virtualVolumeOverhead), imagePath); err != nil {
			_ = os.Remove(imagePath)
			return "", err
		}
		if err := runSystemCommand(ctx, "mkfs.ext4", "-F", "-m", "0", imagePath); err != nil {
			_ = os.Remove(imagePath)
			return "", err
		}
		if err := installVolumeMount(ctx, id, imagePath, dataPath); err != nil {
			return "", err
		}
		return dataPath, nil
	}
	if err != nil {
		return "", err
	}
	currentBytes := stat.Size() - virtualVolumeOverhead
	if currentBytes < 0 {
		currentBytes = 0
	}
	if requestedBytes < currentBytes {
		return "", fmt.Errorf("volumes cannot be reduced below their current size")
	}
	if requestedBytes > currentBytes {
		if err := checkHostSpace(requestedBytes - currentBytes); err != nil {
			return "", err
		}
		if err := runSystemCommand(ctx, "fallocate", "-l", fmt.Sprint(requestedBytes+virtualVolumeOverhead), imagePath); err != nil {
			return "", err
		}
	}
	if err := installVolumeMount(ctx, id, imagePath, dataPath); err != nil {
		return "", err
	}
	loop, err := exec.CommandContext(ctx, "losetup", "-j", imagePath).Output()
	if err != nil {
		return "", fmt.Errorf("find volume loop device: %w", err)
	}
	loopDevice := ""
	if line := strings.SplitN(strings.TrimSpace(string(loop)), "\n", 2)[0]; line != "" {
		loopDevice = strings.SplitN(line, ":", 2)[0]
	}
	if loopDevice == "" {
		return "", fmt.Errorf("virtual volume has no loop device")
	}
	if err := runSystemCommand(ctx, "losetup", "-c", loopDevice); err != nil {
		return "", err
	}
	if err := runSystemCommand(ctx, "resize2fs", loopDevice); err != nil {
		return "", err
	}
	return dataPath, nil
}

func installVolumeMount(ctx context.Context, id, imagePath, dataPath string) error {
	unitName, err := exec.CommandContext(ctx, "systemd-escape", "--path", "--suffix=mount", dataPath).Output()
	if err != nil {
		return fmt.Errorf("create virtual volume mount unit: %w", err)
	}
	unit := strings.TrimSpace(string(unitName))
	if unit == "" || filepath.Base(unit) != unit {
		return fmt.Errorf("invalid virtual volume mount unit")
	}
	unitPath := filepath.Join("/etc/systemd/system", unit)
	content := fmt.Sprintf("[Unit]\nDescription=Control Plane virtual volume %s\nBefore=docker.service\n\n[Mount]\nWhat=%s\nWhere=%s\nType=ext4\nOptions=loop\n\n[Install]\nWantedBy=multi-user.target\n", id, imagePath, dataPath)
	if err := os.WriteFile(unitPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write virtual volume mount unit: %w", err)
	}
	if err := runSystemCommand(ctx, "systemctl", "daemon-reload"); err != nil {
		return err
	}
	return runSystemCommand(ctx, "systemctl", "enable", "--now", unit)
}

func mountUnitName(dataPath string) string {
	name, err := exec.Command("systemd-escape", "--path", "--suffix=mount", dataPath).Output()
	if err != nil {
		return "invalid-volume.mount"
	}
	return strings.TrimSpace(string(name))
}

func checkHostSpace(bytes int64) error {
	var fs syscall.Statfs_t
	if err := syscall.Statfs("/var/lib/control-plane-node", &fs); err != nil {
		return err
	}
	available := int64(fs.Bavail) * int64(fs.Bsize)
	if bytes > available {
		return fmt.Errorf("not enough free disk space on this node")
	}
	return nil
}

func removeVirtualVolume(ctx context.Context, id string) error {
	root, err := virtualVolumeRoot(id)
	if err != nil {
		return err
	}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil
	}
	if runtime.GOOS != "linux" {
		return fmt.Errorf("size-limited volumes require a Linux node")
	}
	dataPath := filepath.Join(root, "data")
	unit := mountUnitName(dataPath)
	if unit == "invalid-volume.mount" {
		return fmt.Errorf("could not identify virtual volume mount")
	}
	_ = runSystemCommand(ctx, "systemctl", "disable", "--now", unit)
	_ = os.Remove(filepath.Join("/etc/systemd/system", unit))
	if err := runSystemCommand(ctx, "systemctl", "daemon-reload"); err != nil {
		return err
	}
	if err := os.RemoveAll(root); err != nil {
		return err
	}
	return nil
}
