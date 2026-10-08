package node

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/gorilla/websocket"
)

type Config struct{ PanelURL, NodeToken, Listen string }

type Agent struct {
	cfg            Config
	docker         *client.Client
	writeMu        sync.Mutex
	consoleMu      sync.Mutex
	consoles       map[string]*consoleSession
	statsMu        sync.Mutex
	serverLocks    sync.Map
	lastCPU        uint64
	lastIdle       uint64
	panelConnected atomic.Bool
}

type consoleSession struct {
	serverID string
	attach   *dockertypes.HijackedResponse
	logs     io.ReadCloser
	write    sync.Mutex
	stop     chan struct{}
}

type Message struct {
	Type        string `json:"type"`
	RequestID   string `json:"requestId,omitempty"`
	ServerID    string `json:"serverId,omitempty"`
	ContainerID string `json:"containerId,omitempty"`
	Command     string `json:"command,omitempty"`
	Payload     any    `json:"payload,omitempty"`
}

func Run(cfg Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, cfg)
}

// NewAgent creates a Docker-connected agent for programmatic use and integration tests.
func NewAgent(cfg Config) (*Agent, error) {
	docker, err := newDockerClient()
	if err != nil {
		return nil, err
	}
	return &Agent{cfg: cfg, docker: docker, consoles: make(map[string]*consoleSession)}, nil
}

// Close releases the Docker client owned by the agent.
func (a *Agent) Close() error { return a.docker.Close() }

// DockerClient exposes the managed client for integration tooling.
func (a *Agent) DockerClient() *client.Client { return a.docker }

func run(ctx context.Context, cfg Config) error {
	a, err := NewAgent(cfg)
	if err != nil {
		return err
	}
	defer a.Close()
	go a.watchPanelConnection(ctx)
	health := &http.Server{Addr: cfg.Listen, Handler: http.HandlerFunc(a.health)}
	go func() {
		if err := health.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logf("node health server: %v", err)
		}
	}()
	for {
		if ctx.Err() != nil {
			break
		}
		if err := a.connect(ctx); err != nil && ctx.Err() == nil {
			logf("panel connection: %v", err)
		}
		select {
		case <-ctx.Done():
		case <-time.After(3 * time.Second):
		}
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := a.stopManagedServers(stopCtx); err != nil {
		logf("stop managed servers during node shutdown: %v", err)
	}
	_ = health.Shutdown(stopCtx)
	return nil
}

// Docker's Go SDK defaults to /var/run/docker.sock, which is commonly absent
// on macOS even when Docker Desktop is running. Prefer Docker Desktop's per-user
// socket there unless the operator explicitly configured DOCKER_HOST.
func newDockerClient() (*client.Client, error) {
	if _, configured := os.LookupEnv("DOCKER_HOST"); runtime.GOOS == "darwin" && !configured {
		if home, err := os.UserHomeDir(); err == nil {
			socket := filepath.Join(home, ".docker", "run", "docker.sock")
			if info, err := os.Stat(socket); err == nil && info.Mode()&os.ModeSocket != 0 {
				return client.NewClientWithOpts(client.FromEnv, client.WithHost("unix://"+socket))
			}
		}
	}
	return client.NewClientWithOpts(client.FromEnv)
}

func logf(format string, args ...any) { fmt.Printf(format+"\n", args...) }

func (a *Agent) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (a *Agent) connect(ctx context.Context) error {
	u := a.cfg.PanelURL
	if strings.HasPrefix(u, "https") {
		u = "wss" + u[5:]
	} else if strings.HasPrefix(u, "http") {
		u = "ws" + u[4:]
	}
	u += "/api/node/connect"
	header := http.Header{}
	header.Set("Authorization", "Bearer "+a.cfg.NodeToken)
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, u, header)
	if err != nil {
		return err
	}
	a.panelConnected.Store(true)
	defer a.panelConnected.Store(false)
	defer conn.Close()
	finished := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-finished:
		}
	}()
	defer close(finished)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
	statuses, statusErr := a.serverStatuses(context.Background())
	if statusErr != nil {
		logf("read managed server states: %v", statusErr)
	}
	if err := a.send(conn, Message{Type: "hello", Payload: map[string]any{"architecture": runtime.GOARCH, "storageMode": storageMode(), "bindIPs": nodeBindIPs(), "serverStatuses": statuses, "time": time.Now()}}); err != nil {
		return err
	}
	statsCtx, stopStats := context.WithCancel(ctx)
	defer stopStats()
	go a.statsLoop(statsCtx, conn)
	for {
		var message Message
		if err := conn.ReadJSON(&message); err != nil {
			return err
		}
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
		switch message.Type {
		case "ping":
			if err := a.send(conn, Message{Type: "pong"}); err != nil {
				return err
			}
		case "create_server", "server_action", "server_volume_create", "server_volume_resize", "server_volume_delete", "server_backup", "server_backup_delete", "server_restore", "server_delete", "server_inspect", "node_resources", "node_firewall_allow", "node_container_delete", "node_image_delete", "file_list", "file_read", "file_write", "file_upload", "file_upload_begin", "file_upload_chunk", "file_upload_finish", "file_upload_abort", "file_delete", "file_upload_url", "file_move", "file_unzip", "file_zip", "file_download_begin", "file_download_chunk", "file_download_finish":
			go a.dispatch(conn, message)
		case "console_attach":
			if err := a.attachConsole(conn, message); err != nil {
				_ = a.send(conn, Message{Type: "console_error", ServerID: message.ServerID, Payload: map[string]string{"text": err.Error()}})
			} else {
				_ = a.send(conn, Message{Type: "console_attached", ServerID: message.ServerID})
			}
		case "console_detach":
			a.detachConsole(message.ServerID)
		case "console_input":
			if err := a.writeConsole(message.ContainerID, message.Command); err != nil {
				_ = a.send(conn, Message{Type: "console_error", ServerID: message.ServerID, Payload: map[string]string{"text": err.Error()}})
			}
		}
	}
}

const panelDisconnectGrace = 30 * time.Second

func (a *Agent) watchPanelConnection(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var disconnectedAt time.Time
	lastStopAttempt := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if a.panelConnected.Load() {
				disconnectedAt = time.Time{}
				lastStopAttempt = time.Time{}
				continue
			}
			if disconnectedAt.IsZero() {
				disconnectedAt = now
				continue
			}
			if now.Sub(disconnectedAt) < panelDisconnectGrace || (!lastStopAttempt.IsZero() && now.Sub(lastStopAttempt) < panelDisconnectGrace) {
				continue
			}
			lastStopAttempt = now
			stopCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
			err := a.stopManagedServers(stopCtx)
			cancel()
			if err != nil {
				logf("stop managed servers after panel disconnect: %v", err)
				continue
			}
			logf("stopped managed servers after panel was unreachable for %s", now.Sub(disconnectedAt).Round(time.Second))
		}
	}
}

func (a *Agent) statsLoop(ctx context.Context, conn *websocket.Conn) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.send(conn, Message{Type: "node_stats", Payload: a.collectStats()}); err != nil {
				return
			}
			statuses, err := a.serverStatuses(ctx)
			if err != nil {
				logf("read managed server states: %v", err)
				continue
			}
			if err := a.send(conn, Message{Type: "server_statuses", Payload: map[string]any{"serverStatuses": statuses}}); err != nil {
				return
			}
		}
	}
}

func (a *Agent) collectStats() map[string]any {
	stats := map[string]any{"cpuThreads": runtime.NumCPU(), "architecture": runtime.GOARCH, "storageMode": storageMode(), "bindIPs": nodeBindIPs(), "kernel": "unknown", "cpuPercent": 0.0, "memoryUsedBytes": uint64(0), "memoryTotalBytes": uint64(0), "storageUsedBytes": uint64(0), "storageTotalBytes": uint64(0)}
	if out, err := exec.Command("uname", "-r").Output(); err == nil {
		stats["kernel"] = strings.TrimSpace(string(out))
	}
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		var total, available uint64
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			v, _ := strconv.ParseUint(fields[1], 10, 64)
			switch fields[0] {
			case "MemTotal:":
				total = v * 1024
			case "MemAvailable:":
				available = v * 1024
			}
		}
		stats["memoryTotalBytes"] = total
		if total > available {
			stats["memoryUsedBytes"] = total - available
		}
	}
	if data, err := os.ReadFile("/proc/stat"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) > 4 && fields[0] == "cpu" {
				var total, idle uint64
				for i, v := range fields[1:] {
					n, _ := strconv.ParseUint(v, 10, 64)
					total += n
					if i == 3 || i == 4 {
						idle += n
					}
				}
				a.statsMu.Lock()
				if a.lastCPU > 0 && total > a.lastCPU {
					delta := total - a.lastCPU
					idleDelta := idle - a.lastIdle
					if delta > 0 {
						stats["cpuPercent"] = 100 * float64(delta-idleDelta) / float64(delta)
					}
				}
				a.lastCPU, a.lastIdle = total, idle
				a.statsMu.Unlock()
				break
			}
		}
	}
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("sysctl", "-n", "hw.memsize").Output(); err == nil {
			if total, parseErr := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64); parseErr == nil {
				stats["memoryTotalBytes"] = total
				if vm, vmErr := exec.Command("vm_stat").Output(); vmErr == nil {
					pageSize := uint64(4096)
					availablePages := uint64(0)
					for i, line := range strings.Split(string(vm), "\n") {
						if i == 0 {
							if fields := strings.Fields(line); len(fields) >= 8 {
								if n, e := strconv.ParseUint(fields[7], 10, 64); e == nil {
									pageSize = n
								}
							}
						}
						fields := strings.Fields(strings.TrimSuffix(strings.TrimSpace(line), "."))
						if len(fields) < 3 {
							continue
						}
						name := strings.TrimSuffix(strings.Join(fields[:2], " "), ":")
						switch name {
						case "Pages free", "Pages inactive", "Pages speculative", "Pages purgeable":
							if n, e := strconv.ParseUint(fields[len(fields)-1], 10, 64); e == nil {
								availablePages += n
							}
						}
					}
					used := uint64(0)
					if availablePages*pageSize < total {
						used = total - availablePages*pageSize
					}
					stats["memoryUsedBytes"] = used
				}
			}
		}
		if out, err := exec.Command("top", "-l", "2", "-n", "0").Output(); err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if !strings.Contains(line, "CPU usage:") {
					continue
				}
				for _, part := range strings.Split(line, ",") {
					fields := strings.Fields(part)
					if len(fields) >= 2 && fields[len(fields)-1] == "idle" {
						idle, _ := strconv.ParseFloat(strings.TrimSuffix(fields[len(fields)-2], "%"), 64)
						stats["cpuPercent"] = 100 - idle
					}
				}
			}
		}
	}
	var fs syscall.Statfs_t
	storagePath := "/var/lib/control-plane-node"
	if runtime.GOOS == "darwin" {
		storagePath = "/"
	}
	if err := syscall.Statfs(storagePath, &fs); err == nil {
		total := fs.Blocks * uint64(fs.Bsize)
		free := fs.Bavail * uint64(fs.Bsize)
		stats["storageTotalBytes"] = total
		if total > free {
			stats["storageUsedBytes"] = total - free
		}
	}
	stats["updatedAt"] = time.Now()
	return stats
}

// nodeBindIPs returns usable host-interface IPs, excluding Docker-managed bridges.
func nodeBindIPs() []string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	addresses := make([]string, 0, len(interfaces))
	seen := make(map[string]bool)
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			addresses = appendUniqueNodeIP(addresses, seen, "127.0.0.1")
			addresses = appendUniqueNodeIP(addresses, seen, "::1")
			continue
		}
		if strings.HasPrefix(iface.Name, "docker") || strings.HasPrefix(iface.Name, "br-") || strings.HasPrefix(iface.Name, "veth") {
			continue
		}
		ifaceAddresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range ifaceAddresses {
			var ip net.IP
			switch value := address.(type) {
			case *net.IPNet:
				ip = value.IP
			case *net.IPAddr:
				ip = value.IP
			}
			if ip == nil || !ip.IsGlobalUnicast() {
				continue
			}
			value := ip.String()
			addresses = appendUniqueNodeIP(addresses, seen, value)
		}
	}
	sort.Slice(addresses, func(i, j int) bool {
		left, right := net.ParseIP(addresses[i]), net.ParseIP(addresses[j])
		if (left.To4() != nil) != (right.To4() != nil) {
			return left.To4() != nil
		}
		return addresses[i] < addresses[j]
	})
	return addresses
}

func appendUniqueNodeIP(addresses []string, seen map[string]bool, ip string) []string {
	if seen[ip] {
		return addresses
	}
	seen[ip] = true
	return append(addresses, ip)
}

func (a *Agent) send(conn *websocket.Conn, message any) error {
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	if err := conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	return conn.WriteJSON(message)
}

func (a *Agent) handleRequest(m Message) (map[string]any, error) {
	payload, _ := m.Payload.(map[string]any)
	timeout := requestTimeout(m, payload)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	switch m.Type {
	case "node_resources":
		return a.dockerResources(ctx)
	case "node_firewall_allow":
		return OpenNodePortAllocations(ctx, payload)
	case "node_container_delete":
		return a.deleteNodeContainer(ctx, payload)
	case "node_image_delete":
		return a.deleteNodeImage(ctx, payload)
	case "server_volume_create", "server_volume_resize":
		id := textValue(payload["volumeId"])
		size := int64(number(payload["sizeBytes"], 0))
		if err := a.prepareServerVolume(ctx, m.Type, id, size); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "id": id, "sizeBytes": size}, nil
	case "server_volume_delete":
		id := textValue(payload["volumeId"])
		if err := a.removeServerVolume(ctx, id); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	case "server_inspect":
		return a.inspectServer(ctx, m)
	case "server_backup_delete":
		return a.deleteBackup(ctx, m, payload)
	case "server_backup":
		return a.createBackup(ctx, m, payload)
	case "server_restore":
		return a.restoreServer(ctx, m, payload)
	case "server_delete":
		return a.deleteServer(ctx, m, payload)
	case "server_action":
		return a.serverAction(ctx, m, payload)
	case "file_list", "file_read", "file_write", "file_upload", "file_upload_begin", "file_upload_chunk", "file_upload_finish", "file_upload_abort", "file_delete", "file_upload_url", "file_move", "file_unzip", "file_zip", "file_download_begin", "file_download_chunk", "file_download_finish":
		return a.handleFileRequest(ctx, m, payload)
	default:
		return nil, fmt.Errorf("unsupported node operation")
	}
}

func requestTimeout(message Message, payload map[string]any) time.Duration {
	switch message.Type {
	case "server_backup", "server_restore", "server_delete", "server_volume_create", "server_volume_resize", "server_volume_delete", "file_unzip", "file_zip", "file_upload", "file_upload_finish", "file_download_begin":
		return 30 * time.Minute
	case "server_action":
		if payload["action"] == "deploy" {
			return 2 * time.Hour
		}
	}
	return 2 * time.Minute
}

func (a *Agent) copyFile(ctx context.Context, id, dst string, data []byte) error {
	name := filepath.Base(dst)
	parent := path.Dir(dst)
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(data)), ModTime: time.Now()}); err != nil {
		return err
	}
	if _, err := tw.Write(data); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return a.docker.CopyToContainer(ctx, id, parent, &b, container.CopyToContainerOptions{
		AllowOverwriteDirWithFile: true,
		CopyUIDGID:                true,
	})
}

// copyFileFromReader streams a tar archive into Docker without buffering the file in memory.
func (a *Agent) copyFileFromReader(ctx context.Context, id, dst string, source io.Reader, size int64) error {
	reader, writer := io.Pipe()
	go func() {
		tarWriter := tar.NewWriter(writer)
		err := tarWriter.WriteHeader(&tar.Header{Name: filepath.Base(dst), Mode: 0644, Size: size, ModTime: time.Now()})
		if err == nil {
			_, err = io.CopyN(tarWriter, source, size)
		}
		if closeErr := tarWriter.Close(); err == nil {
			err = closeErr
		}
		_ = writer.CloseWithError(err)
	}()
	err := a.docker.CopyToContainer(ctx, id, path.Dir(dst), reader, container.CopyToContainerOptions{
		AllowOverwriteDirWithFile: true,
		CopyUIDGID:                true,
	})
	_ = reader.Close()
	return err
}

type backupMount struct {
	Destination string `json:"destination"`
	File        string `json:"file"`
}

func backupRoot() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "control-plane", "backups")
}
func (a *Agent) saveMountedVolumes(ctx context.Context, containerID, serverID, backupID string) (int64, error) {
	inspect, err := a.docker.ContainerInspect(ctx, containerID)
	if err != nil {
		return 0, err
	}
	dir := filepath.Join(backupRoot(), serverID, backupID)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return 0, err
	}
	manifest := make([]backupMount, 0)
	var total int64
	for i, mount := range inspect.Mounts {
		if string(mount.Type) != "volume" && string(mount.Type) != "bind" {
			continue
		}
		archive, _, err := a.docker.CopyFromContainer(ctx, containerID, mount.Destination)
		if err != nil {
			return total, fmt.Errorf("backup mounted path %s: %w", mount.Destination, err)
		}
		filename := fmt.Sprintf("mount-%d.tar", i)
		file, err := os.OpenFile(filepath.Join(dir, filename), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if err != nil {
			_ = archive.Close()
			return total, err
		}
		n, copyErr := io.Copy(file, archive)
		closeErr := file.Close()
		_ = archive.Close()
		if copyErr != nil {
			return total, copyErr
		}
		if closeErr != nil {
			return total, closeErr
		}
		total += n
		manifest = append(manifest, backupMount{Destination: mount.Destination, File: filename})
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return total, err
	}
	if err = os.WriteFile(filepath.Join(dir, "mounts.json"), data, 0600); err != nil {
		return total, err
	}
	return total, nil
}
func (a *Agent) restoreMountedVolumes(ctx context.Context, containerID, serverID, backupID string) error {
	if backupID == "" {
		return fmt.Errorf("backup id missing")
	}
	dir := filepath.Join(backupRoot(), serverID, backupID)
	data, err := os.ReadFile(filepath.Join(dir, "mounts.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var mounts []backupMount
	if err = json.Unmarshal(data, &mounts); err != nil {
		return err
	}
	for _, mount := range mounts {
		clean := filepath.Base(mount.File)
		file, err := os.Open(filepath.Join(dir, clean))
		if err != nil {
			return err
		}
		err = a.docker.CopyToContainer(ctx, containerID, mount.Destination, file, container.CopyToContainerOptions{
			AllowOverwriteDirWithFile: true,
			CopyUIDGID:                true,
		})
		_ = file.Close()
		if err != nil {
			return fmt.Errorf("restore mounted path %s: %w", mount.Destination, err)
		}
	}
	return nil
}

func downloadURL(raw string) ([]byte, error) {
	client := &http.Client{Timeout: 2 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		return validatePublicHost(req.URL.Hostname())
	}}
	if err := validatePublicHost(mustHost(raw)); err != nil {
		return nil, err
	}
	resp, err := client.Get(raw)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
func mustHost(raw string) string {
	u, _ := url.Parse(raw)
	if u == nil {
		return ""
	}
	return u.Hostname()
}
func validatePublicHost(host string) error {
	ips, err := net.LookupIP(host)
	if err != nil {
		return err
	}
	for _, ip := range ips {
		if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast() {
			return fmt.Errorf("URL host resolves to a non-public address")
		}
	}
	return nil
}

func stringSlice(raw any) []string {
	items, _ := raw.([]any)
	if len(items) == 0 {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if value, ok := item.(string); ok {
			result = append(result, value)
		}
	}
	return result
}

func dockerPorts(raw any) (nat.PortSet, nat.PortMap, error) {
	ports := nat.PortSet{}
	bindings := nat.PortMap{}
	items, _ := raw.([]any)
	for _, item := range items {
		portSpec, ok := item.(map[string]any)
		if !ok {
			continue
		}
		containerPort := int(number(portSpec["container"], 0))
		if containerPort < 1 || containerPort > 65535 {
			return nil, nil, fmt.Errorf("Config contains an invalid container port")
		}
		protocol, _ := portSpec["protocol"].(string)
		if protocol == "" {
			protocol = "tcp"
		}
		port, err := nat.NewPort(protocol, strconv.Itoa(containerPort))
		if err != nil {
			return nil, nil, fmt.Errorf("Config contains an invalid port: %w", err)
		}
		hostPort := strings.TrimSpace(fmt.Sprint(portSpec["host"]))
		if hostPort == "<nil>" || hostPort == "" || hostPort == "0" {
			hostPort = strconv.Itoa(containerPort)
		}
		ports[port] = struct{}{}
		hostIP, _ := portSpec["hostIP"].(string)
		if hostIP == "" {
			hostIP = "0.0.0.0"
		}
		bindings[port] = append(bindings[port], nat.PortBinding{HostIP: hostIP, HostPort: hostPort})
	}
	return ports, bindings, nil
}

func number(raw any, fallback float64) float64 {
	switch value := raw.(type) {
	case float64:
		if value > 0 {
			return value
		}
	case int:
		if value > 0 {
			return float64(value)
		}
	case int64:
		if value > 0 {
			return float64(value)
		}
	}
	return fallback
}

func (a *Agent) attachConsole(conn *websocket.Conn, message Message) error {
	if message.ContainerID == "" {
		return fmt.Errorf("server has no container id")
	}
	a.consoleMu.Lock()
	if existing := a.consoles[message.ContainerID]; existing != nil {
		inspect, inspectErr := a.docker.ContainerInspect(context.Background(), message.ContainerID)
		if inspectErr == nil && inspect.State.Running && existing.serverID == message.ServerID {
			a.consoleMu.Unlock()
			return nil
		}
		delete(a.consoles, message.ContainerID)
		close(existing.stop)
		existing.attach.Close()
	}
	attach, err := a.docker.ContainerAttach(context.Background(), message.ContainerID, container.AttachOptions{Stream: true, Stdin: true})
	if err != nil {
		a.consoleMu.Unlock()
		return fmt.Errorf("attach to container: %w", err)
	}
	logs, err := a.docker.ContainerLogs(context.Background(), message.ContainerID, container.LogsOptions{
		ShowStdout: true, ShowStderr: true, Follow: true,
	})
	if err != nil {
		attach.Close()
		a.consoleMu.Unlock()
		return fmt.Errorf("read container logs: %w", err)
	}
	session := &consoleSession{serverID: message.ServerID, attach: &attach, logs: logs, stop: make(chan struct{})}
	a.consoles[message.ContainerID] = session
	a.consoleMu.Unlock()
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, readErr := logs.Read(buffer)
			if n > 0 {
				_ = a.send(conn, Message{Type: "console_output", ServerID: message.ServerID, Payload: map[string]string{"text": string(buffer[:n])}})
			}
			if readErr != nil {
				select {
				case <-session.stop:
				default:
					_ = a.send(conn, Message{Type: "console_error", ServerID: message.ServerID, Payload: map[string]string{"text": readErr.Error()}})
				}
				a.detachConsoleSession(message.ContainerID, session)
				return
			}
			select {
			case <-session.stop:
				return
			default:
			}
		}
	}()
	return nil
}

func (a *Agent) detachConsole(serverID string) {
	a.consoleMu.Lock()
	var session *consoleSession
	for containerID, current := range a.consoles {
		if current != nil && current.serverID == serverID {
			session = current
			delete(a.consoles, containerID)
			break
		}
	}
	if session != nil {
		close(session.stop)
		session.attach.Close()
		_ = session.logs.Close()
	}
	a.consoleMu.Unlock()
}

func (a *Agent) detachConsoleSession(containerID string, session *consoleSession) {
	a.consoleMu.Lock()
	if a.consoles[containerID] == session {
		delete(a.consoles, containerID)
		select {
		case <-session.stop:
		default:
			close(session.stop)
		}
		session.attach.Close()
		_ = session.logs.Close()
	}
	a.consoleMu.Unlock()
}

func (a *Agent) writeConsole(containerID, command string) error {
	a.consoleMu.Lock()
	session := a.consoles[containerID]
	a.consoleMu.Unlock()
	if session == nil {
		return fmt.Errorf("console is not attached")
	}
	session.write.Lock()
	defer session.write.Unlock()
	_, err := session.attach.Conn.Write([]byte(command + "\n"))
	return err
}
