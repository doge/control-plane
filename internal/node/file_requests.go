package node

import (
	"archive/tar"
	"archive/zip"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
)

func (a *Agent) handleFileRequest(ctx context.Context, m Message, payload map[string]any) (map[string]any, error) {
	if m.ContainerID == "" {
		return nil, fmt.Errorf("server has no container id; deploy it first")
	}
	root := textValue(payload["root"])
	if root == "" {
		root = "/data"
	}
	filePath, err := SafePathAt(payload["path"], root)
	if err != nil {
		return nil, err
	}
	switch m.Type {
	case "file_list":
		archive, stat, err := a.docker.CopyFromContainer(ctx, m.ContainerID, filePath)
		if err != nil {
			return nil, err
		}
		defer archive.Close()
		if !stat.Mode.IsDir() {
			return nil, fmt.Errorf("path is not a directory")
		}
		tr := tar.NewReader(archive)
		entries, seen := []map[string]any{}, map[string]bool{}
		target, base := strings.TrimPrefix(filePath, "/"), path.Base(filePath)
		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			name := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(header.Name, "./"), "/"), "/")
			if name == target || name == base || name == "." || name == "" {
				continue
			}
			rel := name
			if strings.HasPrefix(name, target+"/") {
				rel = strings.TrimPrefix(name, target+"/")
			} else if strings.HasPrefix(name, base+"/") {
				rel = strings.TrimPrefix(name, base+"/")
			}
			if rel == "" || strings.Contains(rel, "/") || seen[rel] {
				continue
			}
			seen[rel] = true
			entries = append(entries, map[string]any{"name": rel, "directory": header.FileInfo().IsDir(), "size": header.Size, "modifiedAt": header.ModTime})
		}
		return map[string]any{"entries": entries}, nil
	case "file_read":
		archive, stat, err := a.docker.CopyFromContainer(ctx, m.ContainerID, filePath)
		if err != nil {
			return nil, err
		}
		defer archive.Close()
		if stat.Mode.IsDir() {
			return nil, fmt.Errorf("path is a directory")
		}
		tr := tar.NewReader(archive)
		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			name := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(header.Name, "./"), "/"), "/")
			if header.FileInfo().IsDir() || path.Base(name) != path.Base(filePath) {
				continue
			}
			data, err := io.ReadAll(io.LimitReader(tr, 2*1024*1024+1))
			if err != nil {
				return nil, err
			}
			if len(data) > 2*1024*1024 {
				return nil, fmt.Errorf("file is larger than 2 MB")
			}
			return map[string]any{"content": string(data)}, nil
		}
		return nil, fmt.Errorf("file content was missing from the Docker archive")
	case "file_write":
		content, _ := payload["content"].(string)
		return map[string]any{"ok": true}, a.copyFile(ctx, m.ContainerID, filePath, []byte(content))
	case "file_upload":
		encoded, _ := payload["data"].(string)
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("invalid upload data")
		}
		return map[string]any{"ok": true, "bytes": len(data)}, a.copyFile(ctx, m.ContainerID, filePath, data)
	case "file_delete":
		if filePath == root {
			return nil, fmt.Errorf("cannot delete the server files root")
		}
		response, err := a.docker.ContainerExecCreate(ctx, m.ContainerID, container.ExecOptions{Cmd: []string{"rm", "-rf", "--", filePath}})
		if err != nil {
			return nil, err
		}
		if err := a.docker.ContainerExecStart(ctx, response.ID, container.ExecStartOptions{}); err != nil {
			return nil, err
		}
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			result, err := a.docker.ContainerExecInspect(ctx, response.ID)
			if err != nil {
				return nil, err
			}
			if !result.Running {
				if result.ExitCode != 0 {
					return nil, fmt.Errorf("delete command failed with exit code %d", result.ExitCode)
				}
				return map[string]any{"ok": true}, nil
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-ticker.C:
			}
		}
	case "file_move":
		source, err := SafePathAt(payload["source"], root)
		if err != nil {
			return nil, err
		}
		if source == root {
			return nil, fmt.Errorf("cannot move the server files root")
		}
		destination, err := SafePathAt(payload["destination"], root)
		if err != nil {
			return nil, err
		}
		if destination == root && source == root {
			return nil, fmt.Errorf("invalid move destination")
		}
		if err := runFileCommand(ctx, a, m.ContainerID, "mv", "--", source, strings.TrimSuffix(destination, "/")+"/"); err != nil {
			return nil, fmt.Errorf("move file: %w", err)
		}
		return map[string]any{"ok": true, "destination": destination}, nil
	case "file_unzip":
		archivePath, err := SafePathAt(payload["path"], root)
		if err != nil {
			return nil, err
		}
		if !strings.EqualFold(path.Ext(archivePath), ".zip") {
			return nil, fmt.Errorf("selected file is not a ZIP archive")
		}
		destination := strings.TrimSuffix(archivePath, path.Ext(archivePath))
		if destination == root || path.Base(destination) == "." || path.Base(destination) == "/" {
			return nil, fmt.Errorf("cannot determine extraction folder name")
		}
		if _, err := SafePathAt(destination, root); err != nil {
			return nil, err
		}
		if err := a.extractZip(ctx, m.ContainerID, archivePath, destination, root); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "path": destination}, nil
	case "file_upload_url":
		raw := textValue(payload["url"])
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, fmt.Errorf("only http and https URLs are supported")
		}
		data, err := downloadURL(raw)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "bytes": len(data)}, a.copyFile(ctx, m.ContainerID, filePath, data)
	default:
		return nil, fmt.Errorf("unsupported file operation")
	}
}

func (a *Agent) extractZip(ctx context.Context, containerID, archivePath, destination, root string) (resultErr error) {
	archive, _, err := a.docker.CopyFromContainer(ctx, containerID, archivePath)
	if err != nil {
		return err
	}
	tr := tar.NewReader(archive)
	header, err := tr.Next()
	if err != nil {
		_ = archive.Close()
		return fmt.Errorf("read ZIP file from container: %w", err)
	}
	if header.FileInfo().IsDir() {
		_ = archive.Close()
		return fmt.Errorf("ZIP path is a directory")
	}
	tmp, err := os.CreateTemp("", "control-plane-*.zip")
	if err != nil {
		_ = archive.Close()
		return err
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	if _, err = io.CopyN(tmp, tr, header.Size); err != nil {
		_ = archive.Close()
		return fmt.Errorf("read ZIP file from container: %w", err)
	}
	if err := archive.Close(); err != nil {
		return err
	}
	files, err := zip.NewReader(tmp, header.Size)
	if err != nil {
		return fmt.Errorf("invalid ZIP archive: %w", err)
	}

	if err := runFileCommand(ctx, a, containerID, "mkdir", "--", destination); err != nil {
		return fmt.Errorf("create extraction folder: %w", err)
	}
	created := true
	defer func() {
		if resultErr != nil && created {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = runFileCommand(cleanupCtx, a, containerID, "rm", "-rf", "--", destination)
		}
	}()

	var directories []string
	for _, file := range files.File {
		name := strings.TrimSuffix(file.Name, "/")
		if name == "" {
			continue
		}
		if path.IsAbs(name) || strings.Contains(name, "\\") {
			return fmt.Errorf("ZIP contains an unsafe path")
		}
		clean := path.Clean(name)
		if clean == "." && file.FileInfo().IsDir() {
			continue
		}
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
			return fmt.Errorf("ZIP contains an unsafe path")
		}
		target, err := SafePathAt(path.Join(destination, clean), root)
		if err != nil {
			return fmt.Errorf("ZIP contains an unsafe path: %w", err)
		}
		mode := file.Mode()
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("ZIP archives containing symbolic links are not supported")
		}
		if file.FileInfo().IsDir() {
			directories = append(directories, target)
			continue
		}
		if !mode.IsRegular() {
			return fmt.Errorf("ZIP archive contains an unsupported special file")
		}
		for parent := path.Dir(target); parent != destination && strings.HasPrefix(parent, destination+"/"); parent = path.Dir(parent) {
			directories = append(directories, parent)
		}
	}
	if err := makeZipDirectories(ctx, a, containerID, directories); err != nil {
		return err
	}
	for _, file := range files.File {
		if file.FileInfo().IsDir() {
			continue
		}
		name := strings.TrimSuffix(file.Name, "/")
		target := path.Join(destination, path.Clean(name))
		rc, err := file.Open()
		if err != nil {
			return err
		}
		mode := int64(file.Mode().Perm())
		if mode == 0 {
			mode = 0644
		}
		err = a.copyFileReader(ctx, containerID, target, rc, int64(file.UncompressedSize64), mode)
		_ = rc.Close()
		if err != nil {
			return fmt.Errorf("extract %s: %w", name, err)
		}
	}
	created = false
	return nil
}

func makeZipDirectories(ctx context.Context, a *Agent, containerID string, directories []string) error {
	seen := make(map[string]bool, len(directories))
	unique := directories[:0]
	for _, dir := range directories {
		if !seen[dir] {
			seen[dir] = true
			unique = append(unique, dir)
		}
	}
	for len(unique) > 0 {
		n := len(unique)
		if n > 64 {
			n = 64
		}
		args := append([]string{"mkdir", "-p", "--"}, unique[:n]...)
		if err := runFileCommand(ctx, a, containerID, args...); err != nil {
			return fmt.Errorf("create ZIP subfolder: %w", err)
		}
		unique = unique[n:]
	}
	return nil
}

func runFileCommand(ctx context.Context, a *Agent, containerID string, command ...string) error {
	response, err := a.docker.ContainerExecCreate(ctx, containerID, container.ExecOptions{Cmd: command})
	if err != nil {
		return err
	}
	if err := a.docker.ContainerExecStart(ctx, response.ID, container.ExecStartOptions{}); err != nil {
		return err
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		result, err := a.docker.ContainerExecInspect(ctx, response.ID)
		if err != nil {
			return err
		}
		if !result.Running {
			if result.ExitCode != 0 {
				return fmt.Errorf("%s failed with exit code %d", command[0], result.ExitCode)
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (a *Agent) copyFileReader(ctx context.Context, containerID, destination string, source io.Reader, size, mode int64) error {
	reader, writer := io.Pipe()
	writerDone := make(chan error, 1)
	go func() {
		archive := tar.NewWriter(writer)
		err := archive.WriteHeader(&tar.Header{Name: path.Base(destination), Mode: mode, Size: size, ModTime: time.Now()})
		if err == nil {
			_, err = io.CopyN(archive, source, size)
		}
		if closeErr := archive.Close(); err == nil {
			err = closeErr
		}
		_ = writer.CloseWithError(err)
		writerDone <- err
	}()
	copyErr := a.docker.CopyToContainer(ctx, containerID, path.Dir(destination), reader, container.CopyToContainerOptions{AllowOverwriteDirWithFile: true})
	if copyErr != nil {
		_ = reader.CloseWithError(copyErr)
	}
	writeErr := <-writerDone
	if copyErr != nil {
		return copyErr
	}
	return writeErr
}

// SafePathAt validates a path and resolves it within the configured data root.
func SafePathAt(raw any, root string) (string, error) {
	if strings.Contains(root, "\\") || !strings.HasPrefix(root, "/") {
		return "", fmt.Errorf("invalid server data directory")
	}
	root = path.Clean(root)
	if root == "/" || strings.Contains(root, "\x00") {
		return "", fmt.Errorf("invalid server data directory")
	}
	value, _ := raw.(string)
	if value == "" || value == "/" {
		return root, nil
	}
	if strings.Contains(value, "\\") || strings.Contains(value, "\x00") {
		return "", fmt.Errorf("invalid path")
	}
	if !strings.HasPrefix(value, "/") {
		value = path.Join(root, value)
	}
	value = path.Clean(value)
	if value != root && !strings.HasPrefix(value, root+"/") {
		return "", fmt.Errorf("invalid path")
	}
	return value, nil
}
