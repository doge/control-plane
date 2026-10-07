package controller

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/example/control-plane/internal/config"
	"github.com/example/control-plane/internal/models"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func (c *Controller) serverOps(w http.ResponseWriter, r *http.Request) {
	part := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/servers/"), "/")
	bits := strings.Split(part, "/")
	if len(bits) < 1 {
		http.NotFound(w, r)
		return
	}
	id, err := bson.ObjectIDFromHex(bits[0])
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid server id"})
		return
	}
	permission := ""
	if len(bits) == 1 {
		if r.Method == http.MethodDelete {
			permission = "servers.delete"
		} else {
			http.NotFound(w, r)
			return
		}
	} else {
		switch bits[1] {
		case "console":
			permission = "servers.console"
		case "actions":
			permission = "servers.control"
		case "config":
			permission = "servers.view"
			if r.Method != http.MethodGet {
				permission = "servers.update"
			}
		case "launch":
			permission = "servers.view"
		case "backups":
			permission = "servers.backups.view"
			if r.Method != http.MethodGet {
				permission = "servers.backups.manage"
			}
		case "files":
			permission = "servers.files.view"
			if r.Method != http.MethodGet {
				permission = "servers.files.manage"
			}
		case "volumes":
			permission = "servers.update"
			if r.Method == http.MethodGet {
				permission = "servers.view"
			}
			if len(bits) >= 4 && bits[3] == "files" {
				permission = "volumes.files.view"
				if r.Method != http.MethodGet {
					permission = "volumes.files.manage"
				}
			}
			if len(bits) == 3 && r.Method == http.MethodPatch {
				permission = "volumes.extend"
			}
		default:
			http.NotFound(w, r)
			return
		}
	}
	if !requirePermission(w, r, permission) {
		return
	}
	var server models.Server
	if err = c.service.Servers.FindOne(r.Context(), bson.M{"_id": id}).Decode(&server); err != nil {
		writeJSON(w, 404, map[string]string{"error": "server not found"})
		return
	}
	if len(bits) == 1 {
		if r.Method == http.MethodDelete {
			c.deleteServer(w, r, server)
			return
		}
		http.NotFound(w, r)
		return
	}
	if bits[1] == "console" {
		c.app.serverConsole(w, r)
		return
	}
	if bits[1] == "actions" {
		c.serverAction(w, r, server)
		return
	}
	if bits[1] == "config" {
		c.updateServerConfig(w, r, server)
		return
	}
	if bits[1] == "launch" {
		c.serverLaunchInfo(w, r, server)
		return
	}
	if bits[1] == "backups" {
		c.serverBackups(w, r, server, bits[2:])
		return
	}
	if bits[1] == "files" {
		c.serverFiles(w, r, server, bits[2:])
		return
	}
	if bits[1] == "volumes" {
		if !volumeServerVisible(r, server) {
			http.NotFound(w, r)
			return
		}
		if len(bits) >= 4 && bits[3] == "files" {
			c.serverVolumeFiles(w, r, server, bits[2], bits[4:])
			return
		}
		c.serverVolumes(w, r, server, bits[2:])
		return
	}
	http.NotFound(w, r)
}

func (c *Controller) updateServerConfig(w http.ResponseWriter, r *http.Request, s models.Server) {
	if r.Method != http.MethodPut && r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		CPULimit      float64                 `json:"cpuLimit"`
		MemoryMB      int64                   `json:"memoryMB"`
		Variables     map[string]string       `json:"variables"`
		Allocations   []models.PortAllocation `json:"allocations"`
		LaunchCommand string                  `json:"launchCommand"`
	}
	if r.Method == http.MethodPut {
		if bodyJSON(r, &in) != nil || in.CPULimit <= 0 || in.MemoryMB < 256 || in.MemoryMB > 1048576 {
			writeJSON(w, 400, map[string]string{"error": "valid CPU and memory limits are required"})
			return
		}
	}
	var node models.Node
	if err := c.service.Nodes.FindOne(r.Context(), bson.M{"_id": s.NodeID}).Decode(&node); err != nil {
		writeJSON(w, 404, map[string]string{"error": "server node not found"})
		return
	}
	var value models.Config
	if err := c.service.Configs.FindOne(r.Context(), bson.M{"_id": s.ConfigID}).Decode(&value); err != nil {
		writeJSON(w, 404, map[string]string{"error": "server Config not found"})
		return
	}
	c.hydrateNodeNetwork(r.Context(), &node)
	if r.Method == http.MethodGet {
		if len(s.Allocations) == 0 {
			allocations, address, allocationErr := c.allocatePorts(r.Context(), node, value)
			if allocationErr == nil {
				s.Allocations, s.Address = allocations, address
			}
		}
		s.Variables = publicServerVariables(value, s.Variables)
		writeJSON(w, 200, s)
		return
	}
	used := map[string]bool{}
	var all []models.Server
	if err := c.service.Servers.FindAll(r.Context(), bson.M{"nodeId": node.ID}, &all); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	for _, other := range all {
		if other.ID == s.ID || other.Status == "error" {
			continue
		}
		for _, al := range other.Allocations {
			used[fmt.Sprintf("%s:%d/*", al.IP, al.Host)] = true
		}
	}
	allocations := make([]models.PortAllocation, 0, len(in.Allocations))
	current := map[string]bool{}
	for _, raw := range in.Allocations {
		containerPort := raw.Container
		protocol := strings.ToLower(strings.TrimSpace(raw.Protocol))
		if protocol == "" {
			protocol = "tcp"
		}
		if containerPort < 1 || containerPort > 65535 || (protocol != "tcp" && protocol != "udp" && protocol != "sctp") || raw.Host < 1 || raw.Host > 65535 {
			writeJSON(w, 400, map[string]string{"error": "each allocation needs a valid container port, host port, and tcp/udp/sctp protocol"})
			return
		}
		allowed := false
		for _, allocation := range node.PortAllocations {
			if raw.IP == allocation.IP && raw.Host == allocation.Port {
				allowed = true
				break
			}
		}
		if !allowed {
			writeJSON(w, 400, map[string]string{"error": fmt.Sprintf("%s:%d is not an allocated port on this node", raw.IP, raw.Host)})
			return
		}
		key := fmt.Sprintf("%s:%d/%s", raw.IP, raw.Host, protocol)
		if used[fmt.Sprintf("%s:%d/*", raw.IP, raw.Host)] || current[key] {
			writeJSON(w, 409, map[string]string{"error": fmt.Sprintf("%s:%d is already allocated to another server", raw.IP, raw.Host)})
			return
		}
		current[key] = true
		allocations = append(allocations, models.PortAllocation{Name: strings.TrimSpace(raw.Name), IP: raw.IP, Host: raw.Host, Container: containerPort, Protocol: protocol})
	}
	if s.Variables == nil {
		s.Variables = map[string]string{}
	}
	secretVariables := map[string]bool{}
	for _, definition := range value.Spec.Variables {
		if definition.Secret || definition.Type == "secret" {
			secretVariables[definition.Name] = true
			secretVariables[strings.ToUpper(definition.Name)] = true
		}
	}
	for key, value := range in.Variables {
		in.Variables[key] = strings.TrimSpace(value)
	}
	for key, value := range in.Variables {
		if secretVariables[key] && value == "" {
			continue
		}
		s.Variables[key] = value
	}
	if resolved, err := config.DefaultValues(value.Spec, s.Variables); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	} else {
		s.Variables = resolved
	}
	s.CPULimit, s.MemoryMB, s.Allocations, s.LaunchCommand, s.GameCommand = in.CPULimit, in.MemoryMB, allocations, strings.TrimSpace(in.LaunchCommand), ""
	if len(allocations) > 0 {
		s.Address = allocations[0].IP
	} else {
		ips := node.IPs
		if len(ips) == 0 {
			ips, _ = resolveAllIPs(node.Address)
		}
		if len(ips) > 0 {
			s.Address = ips[0]
		}
	}
	if s.ContainerID != "" {
		n, ok := c.nodeForServer(w, r, s)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Hour)
		defer cancel()
		payload := BuildServerCreatePayload(s, value)
		image, _ := payload["image"].(string)
		if strings.TrimSpace(image) == "" {
			writeJSON(w, 400, map[string]string{"error": "Config has no Docker image"})
			return
		}
		payload["action"] = "deploy"
		result, e := n.request(ctx, map[string]any{"type": "server_action", "serverId": s.ID.Hex(), "containerId": s.ContainerID, "payload": payload})
		if e != nil {
			s.ContainerID, s.Status, s.UpdatedAt = "", "error", time.Now()
			_, _ = c.service.Servers.UpdateOne(context.Background(), bson.M{"_id": s.ID}, bson.M{"$set": bson.M{
				"cpuLimit": s.CPULimit, "memoryMB": s.MemoryMB, "variables": s.Variables,
				"allocations": s.Allocations, "address": s.Address, "launchCommand": s.LaunchCommand,
				"containerId": "", "status": s.Status, "updatedAt": s.UpdatedAt,
			}})
			writeJSON(w, 502, map[string]string{"error": e.Error()})
			return
		}
		p, _ := result["payload"].(map[string]any)
		s.ContainerID, _ = p["containerId"].(string)
		s.Status = "running"
	}
	s.UpdatedAt = time.Now()
	if _, err := c.service.Servers.UpdateOne(r.Context(), bson.M{"_id": s.ID}, bson.M{"$set": bson.M{"cpuLimit": s.CPULimit, "memoryMB": s.MemoryMB, "variables": s.Variables, "allocations": s.Allocations, "address": s.Address, "launchCommand": s.LaunchCommand, "gameCommand": "", "containerId": s.ContainerID, "status": s.Status, "updatedAt": s.UpdatedAt}}); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.Variables = publicServerVariables(value, s.Variables)
	c.recordAudit(r, "server.config.updated", s.Name)
	writeJSON(w, 200, s)
}

func (c *Controller) serverLaunchInfo(w http.ResponseWriter, r *http.Request, s models.Server) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var value models.Config
	if err := c.service.Configs.FindOne(r.Context(), bson.M{"_id": s.ConfigID}).Decode(&value); err != nil {
		writeJSON(w, 404, map[string]string{"error": "server Config not found"})
		return
	}
	image := ""
	if len(value.Spec.DockerImages) > 0 {
		image = value.Spec.DockerImages[0]
	}
	info := map[string]any{"launchCommand": s.LaunchCommand, "gameCommand": s.GameCommand, "configCommand": value.Spec.Startup, "image": image}
	if s.ContainerID != "" {
		c.app.nodesMu.RLock()
		n := c.app.nodeSessions[s.NodeID]
		c.app.nodesMu.RUnlock()
		if n != nil {
			result, err := n.request(r.Context(), map[string]any{"type": "server_inspect", "serverId": s.ID.Hex(), "containerId": s.ContainerID, "payload": map[string]any{}})
			if err == nil {
				if payload, ok := result["payload"].(map[string]any); ok {
					for key, value := range payload {
						info[key] = value
					}
					if command, ok := payload["gameCommand"].(string); ok && command != "" {
						info["gameCommand"] = command
						_, _ = c.service.Servers.UpdateOne(r.Context(), bson.M{"_id": s.ID}, bson.M{"$set": bson.M{"gameCommand": command}})
					} else {
						delete(info, "gameCommand")
						_, _ = c.service.Servers.UpdateOne(r.Context(), bson.M{"_id": s.ID}, bson.M{"$unset": bson.M{"gameCommand": ""}})
					}
				}
			}
		}
	}
	writeJSON(w, 200, info)
}

func (c *Controller) deleteServer(w http.ResponseWriter, r *http.Request, s models.Server) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", 405)
		return
	}
	n, ok := c.nodeForServer(w, r, s)
	if !ok {
		return
	}
	var backups []models.Backup
	if err := c.service.Backups.FindAll(r.Context(), bson.M{"serverId": s.ID}, &backups); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	images := make([]any, 0, len(backups))
	for _, b := range backups {
		images = append(images, b.Image)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	volumeIDs := make([]string, 0, len(s.Volumes))
	for _, volume := range s.Volumes {
		volumeIDs = append(volumeIDs, volume.ID)
	}
	if _, err := n.request(ctx, map[string]any{"type": "server_delete", "serverId": s.ID.Hex(), "containerId": s.ContainerID, "payload": map[string]any{"backupImages": images, "volumeIds": volumeIDs}}); err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	_, _ = c.service.Backups.DeleteMany(r.Context(), bson.M{"serverId": s.ID})
	_, _ = c.service.Servers.DeleteOne(r.Context(), bson.M{"_id": s.ID})
	c.recordAudit(r, "server.deleted", s.Name)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (c *Controller) serverBackups(w http.ResponseWriter, r *http.Request, s models.Server, tail []string) {
	if len(tail) == 0 && r.Method == http.MethodGet {
		out := make([]models.Backup, 0)
		if err := c.service.Backups.FindAll(r.Context(), bson.M{"serverId": s.ID}, &out); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, out)
		return
	}
	if len(tail) == 0 && r.Method == http.MethodPost {
		if s.ContainerID == "" {
			writeJSON(w, 409, map[string]string{"error": "deploy the server before creating a backup"})
			return
		}
		n, ok := c.nodeForServer(w, r, s)
		if !ok {
			return
		}
		backupID := randomToken(12)
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
		defer cancel()
		result, err := n.request(ctx, map[string]any{"type": "server_backup", "serverId": s.ID.Hex(), "containerId": s.ContainerID, "payload": map[string]any{"backupId": backupID}})
		if err != nil {
			writeJSON(w, 502, map[string]string{"error": err.Error()})
			return
		}
		payload, _ := result["payload"].(map[string]any)
		backup := models.Backup{ID: backupID, ServerID: s.ID, NodeID: s.NodeID, CreatedAt: time.Now()}
		backup.Image, _ = payload["image"].(string)
		if size, ok := payload["sizeBytes"].(float64); ok {
			backup.SizeBytes = int64(size)
		}
		if _, err = c.service.Backups.InsertOne(r.Context(), backup); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		c.recordAudit(r, "server.backup.created", s.Name)
		writeJSON(w, 201, backup)
		return
	}
	if len(tail) == 1 && r.Method == http.MethodDelete {
		var backup models.Backup
		if err := c.service.Backups.FindOne(r.Context(), bson.M{"_id": tail[0], "serverId": s.ID}).Decode(&backup); err != nil {
			writeJSON(w, 404, map[string]string{"error": "backup not found"})
			return
		}
		n, ok := c.nodeForServer(w, r, s)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
		defer cancel()
		if _, err := n.request(ctx, map[string]any{"type": "server_backup_delete", "serverId": s.ID.Hex(), "payload": map[string]any{"backupId": backup.ID}}); err != nil {
			writeJSON(w, 502, map[string]string{"error": err.Error()})
			return
		}
		if _, err := c.service.Backups.DeleteOne(r.Context(), bson.M{"_id": backup.ID, "serverId": s.ID}); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		c.recordAudit(r, "server.backup.deleted", s.Name)
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	if len(tail) == 2 && tail[1] == "restore" && r.Method == http.MethodPost {
		var backup models.Backup
		if err := c.service.Backups.FindOne(r.Context(), bson.M{"_id": tail[0], "serverId": s.ID}).Decode(&backup); err != nil {
			writeJSON(w, 404, map[string]string{"error": "backup not found"})
			return
		}
		n, ok := c.nodeForServer(w, r, s)
		if !ok {
			return
		}
		var value models.Config
		if err := c.service.Configs.FindOne(r.Context(), bson.M{"_id": s.ConfigID}).Decode(&value); err != nil {
			writeJSON(w, 404, map[string]string{"error": "server Config not found"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
		defer cancel()
		result, err := n.request(ctx, map[string]any{"type": "server_restore", "serverId": s.ID.Hex(), "containerId": s.ContainerID, "payload": map[string]any{"image": backup.Image, "backupId": backup.ID, "config": BuildServerCreatePayload(s, value)}})
		if err != nil {
			writeJSON(w, 502, map[string]string{"error": err.Error()})
			return
		}
		payload, _ := result["payload"].(map[string]any)
		s.ContainerID, _ = payload["containerId"].(string)
		s.Status = "running"
		s.UpdatedAt = time.Now()
		_, _ = c.service.Servers.UpdateOne(r.Context(), bson.M{"_id": s.ID}, bson.M{"$set": bson.M{"containerId": s.ContainerID, "status": s.Status, "updatedAt": s.UpdatedAt}})
		c.recordAudit(r, "server.backup.restored", s.Name)
		if err := c.redactServerVariables(r.Context(), []models.Server{s}); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, s)
		return
	}
	writeJSON(w, 405, map[string]string{"error": "method not allowed"})
}

func (c *Controller) nodeForServer(w http.ResponseWriter, r *http.Request, s models.Server) (*nodeSession, bool) {
	c.app.nodesMu.RLock()
	n := c.app.nodeSessions[s.NodeID]
	c.app.nodesMu.RUnlock()
	if n == nil {
		writeJSON(w, 409, map[string]string{"error": "node is offline"})
		return nil, false
	}
	return n, true
}
func (c *Controller) serverAction(w http.ResponseWriter, r *http.Request, s models.Server) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var in struct {
		Action string `json:"action"`
	}
	if bodyJSON(r, &in) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request"})
		return
	}
	if in.Action != "deploy" && in.Action != "start" && in.Action != "pause" && in.Action != "resume" && in.Action != "kill" {
		writeJSON(w, 400, map[string]string{"error": "unsupported action"})
		return
	}
	n, ok := c.nodeForServer(w, r, s)
	if !ok {
		return
	}
	timeout := 10 * time.Minute
	if in.Action == "deploy" {
		timeout = 2 * time.Hour
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	message := map[string]any{"type": "server_action", "serverId": s.ID.Hex(), "containerId": s.ContainerID, "payload": map[string]any{"action": in.Action}}
	if in.Action == "deploy" {
		var value models.Config
		if err := c.service.Configs.FindOne(r.Context(), bson.M{"_id": s.ConfigID}).Decode(&value); err != nil {
			writeJSON(w, 404, map[string]string{"error": "server Config not found"})
			return
		}
		message["type"] = "server_action"
		payload := BuildServerCreatePayload(s, value)
		image, _ := payload["image"].(string)
		if strings.TrimSpace(image) == "" {
			writeJSON(w, 400, map[string]string{"error": "Config has no Docker image; add one to spec.dockerImages"})
			return
		}
		payload["action"] = "deploy"
		message["payload"] = payload
	}
	result, err := n.request(ctx, message)
	if err != nil {
		if in.Action == "deploy" {
			s.ContainerID, s.Status, s.UpdatedAt = "", "error", time.Now()
			_, _ = c.service.Servers.UpdateOne(context.Background(), bson.M{"_id": s.ID}, bson.M{"$set": bson.M{"containerId": "", "status": s.Status, "updatedAt": s.UpdatedAt}})
		}
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	payload, _ := result["payload"].(map[string]any)
	status, _ := payload["status"].(string)
	if in.Action == "deploy" {
		s.ContainerID, _ = payload["containerId"].(string)
		s.GameCommand = ""
		status = "running"
	}
	if status != "" {
		s.Status = status
		s.UpdatedAt = time.Now()
		update := bson.M{"status": status, "containerId": s.ContainerID, "updatedAt": s.UpdatedAt}
		if in.Action == "deploy" {
			update["gameCommand"] = ""
		}
		_, _ = c.service.Servers.UpdateOne(r.Context(), bson.M{"_id": s.ID}, bson.M{"$set": update})
	}
	c.recordAudit(r, "server."+in.Action, s.Name)
	if err := c.redactServerVariables(r.Context(), []models.Server{s}); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, s)
}
func (c *Controller) fileRequest(w http.ResponseWriter, r *http.Request, s models.Server, typ, filePath string, payload map[string]any) {
	if s.ContainerID == "" {
		writeJSON(w, 409, map[string]string{"error": "server has no container id; deploy it first"})
		return
	}
	n, ok := c.nodeForServer(w, r, s)
	if !ok {
		return
	}
	timeout := 3 * time.Minute
	if typ == "file_unzip" {
		timeout = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	result, err := n.request(ctx, map[string]any{"type": typ, "serverId": s.ID.Hex(), "containerId": s.ContainerID, "payload": payload})
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	p, _ := result["payload"].(map[string]any)
	if p == nil {
		p = map[string]any{}
	}
	writeJSON(w, 200, p)
}
func (c *Controller) serverFiles(w http.ResponseWriter, r *http.Request, s models.Server, tail []string) {
	root := "/home/container"
	var value models.Config
	if c.service.Configs.FindOne(r.Context(), bson.M{"_id": s.ConfigID}).Decode(&value) == nil && value.Spec.DataDirectory != "" {
		root = value.Spec.DataDirectory
	}
	c.serverFilesAtRoot(w, r, s, tail, root)
}

func (c *Controller) serverVolumeFiles(w http.ResponseWriter, r *http.Request, s models.Server, volumeID string, tail []string) {
	root := ""
	if volumeID == "data" {
		root = "/home/container"
		var value models.Config
		if c.service.Configs.FindOne(r.Context(), bson.M{"_id": s.ConfigID}).Decode(&value) == nil && value.Spec.DataDirectory != "" {
			root = value.Spec.DataDirectory
		}
	} else {
		for _, volume := range s.Volumes {
			if volume.ID == volumeID {
				root = volume.MountPath
				break
			}
		}
	}
	if root == "" {
		http.NotFound(w, r)
		return
	}
	c.serverFilesAtRoot(w, r, s, tail, root)
}

func (c *Controller) serverFilesAtRoot(w http.ResponseWriter, r *http.Request, s models.Server, tail []string, root string) {
	request := func(kind, destination string, payload map[string]any) {
		payload["root"] = root
		c.fileRequest(w, r, s, kind, destination, payload)
	}
	filePath := r.URL.Query().Get("path")
	if filePath == "" || filePath == "/" {
		filePath = root
	}
	if len(tail) > 0 {
		switch tail[0] {
		case "content":
			if r.Method == http.MethodGet {
				request("file_read", filePath, map[string]any{"path": filePath})
				return
			}
			if r.Method == http.MethodPut {
				var in struct {
					Content string `json:"content"`
				}
				if bodyJSON(r, &in) != nil {
					writeJSON(w, 400, map[string]string{"error": "invalid content"})
					return
				}
				request("file_write", filePath, map[string]any{"path": filePath, "content": in.Content})
				return
			}
		case "upload":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", 405)
				return
			}
			if err := r.ParseMultipartForm(32 << 20); err != nil {
				writeJSON(w, 400, map[string]string{"error": "invalid multipart upload"})
				return
			}
			if r.MultipartForm != nil {
				defer r.MultipartForm.RemoveAll()
			}
			f, h, err := r.FormFile("file")
			if err != nil {
				writeJSON(w, 400, map[string]string{"error": "select a file"})
				return
			}
			defer f.Close()
			data, err := io.ReadAll(f)
			if err != nil {
				writeJSON(w, 500, map[string]string{"error": "could not read uploaded file"})
				return
			}
			dst := r.FormValue("path")
			if dst == "" {
				dst = path.Join(root, h.Filename)
			}
			request("file_upload", dst, map[string]any{"path": dst, "data": base64.StdEncoding.EncodeToString(data)})
			return
		case "from-url":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", 405)
				return
			}
			var in struct {
				URL  string `json:"url"`
				Path string `json:"path"`
			}
			if bodyJSON(r, &in) != nil || strings.TrimSpace(in.URL) == "" || strings.TrimSpace(in.Path) == "" {
				writeJSON(w, 400, map[string]string{"error": "url and path are required"})
				return
			}
			request("file_upload_url", in.Path, map[string]any{"url": in.URL, "path": in.Path})
			return
		case "delete":
			if r.Method != http.MethodDelete {
				http.Error(w, "method not allowed", 405)
				return
			}
			request("file_delete", filePath, map[string]any{"path": filePath})
			return
		case "move":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", 405)
				return
			}
			var in struct {
				Source      string `json:"source"`
				Destination string `json:"destination"`
			}
			if bodyJSON(r, &in) != nil || strings.TrimSpace(in.Source) == "" || strings.TrimSpace(in.Destination) == "" {
				writeJSON(w, 400, map[string]string{"error": "source and destination folder are required"})
				return
			}
			request("file_move", in.Source, map[string]any{"source": in.Source, "destination": in.Destination})
			return
		case "unzip":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", 405)
				return
			}
			var in struct {
				Path string `json:"path"`
			}
			if bodyJSON(r, &in) != nil || strings.TrimSpace(in.Path) == "" {
				writeJSON(w, 400, map[string]string{"error": "zip file path is required"})
				return
			}
			request("file_unzip", in.Path, map[string]any{"path": in.Path})
			return
		}
	}
	if r.Method == http.MethodGet {
		request("file_list", filePath, map[string]any{"path": filePath})
		return
	}
	writeJSON(w, 405, map[string]string{"error": "method not allowed"})
}

func volumeServerVisible(r *http.Request, server models.Server) bool {
	actor, _ := r.Context().Value(userKey).(models.User)
	return actor.Role == "root" || (!server.ManagerID.IsZero() && server.ManagerID == actor.ID)
}
