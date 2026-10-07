package controller

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/example/control-plane/internal/config"
	"github.com/example/control-plane/internal/models"
	"github.com/example/control-plane/internal/panel/service"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func resolveHost(value string) (string, error) {
	host := strings.TrimSpace(strings.TrimSuffix(value, "."))
	if host == "" {
		return "", fmt.Errorf("address required")
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String(), nil
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", fmt.Errorf("invalid hostname")
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return "", fmt.Errorf("invalid hostname")
			}
		}
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return "", fmt.Errorf("hostname lookup failed")
	}
	return host, nil
}
func resolveAllIPs(host string) ([]string, error) {
	ip := net.ParseIP(host)
	if ip != nil {
		return []string{ip.String()}, nil
	}
	resolved, err := net.LookupIP(host)
	if err != nil || len(resolved) == 0 {
		return nil, fmt.Errorf("hostname lookup failed")
	}
	out := make([]string, 0, len(resolved))
	seen := map[string]bool{}
	for _, candidate := range resolved {
		text := candidate.String()
		if net.ParseIP(text) == nil || seen[text] {
			continue
		}
		out = append(out, text)
		seen[text] = true
	}
	sort.Slice(out, func(i, j int) bool { return net.ParseIP(out[i]).To4() != nil && net.ParseIP(out[j]).To4() == nil })
	if len(out) == 0 {
		return nil, fmt.Errorf("hostname has no valid IP records")
	}
	return out, nil
}

func (c *Controller) hydrateNodeNetwork(ctx context.Context, node *models.Node) {
	if node == nil {
		return
	}
	ips := node.IPs
	if len(ips) == 0 {
		resolved, err := resolveAllIPs(node.Address)
		if err != nil {
			return
		}
		ips = resolved
	}
	if len(ips) == len(node.IPs) {
		return
	}
	node.IPs = ips
	_, _ = c.service.Nodes.UpdateOne(ctx, bson.M{"_id": node.ID}, bson.M{"$set": bson.M{"ips": ips}})
}

func mergeIPs(groups ...[]string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, group := range groups {
		for _, ip := range group {
			if !seen[ip] {
				seen[ip] = true
				out = append(out, ip)
			}
		}
	}
	return out
}
func validateIPAddresses(values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		host, err := resolveHost(value)
		if err != nil {
			return nil, fmt.Errorf("allocation address %q must resolve to a valid IP", value)
		}
		ips, err := net.LookupIP(host)
		if err != nil || len(ips) == 0 {
			return nil, fmt.Errorf("allocation address %q did not resolve", value)
		}
		ip := ips[0].String()
		if parsed := net.ParseIP(host); parsed != nil {
			ip = parsed.String()
		}
		if !seen[ip] {
			result = append(result, ip)
			seen[ip] = true
		}
	}
	return result, nil
}
func (c *Controller) allocatePorts(ctx context.Context, node models.Node, value models.Config) ([]models.PortAllocation, string, error) {
	ports := value.Spec.Ports
	ips := node.IPs
	if len(ips) == 0 {
		resolved, err := resolveAllIPs(node.Address)
		if err != nil || len(resolved) == 0 {
			return nil, "", fmt.Errorf("node address no longer resolves to an IP")
		}
		ips = resolved
	}
	available := node.PortAllocations
	if len(available) == 0 && len(ports) > 0 {
		return nil, "", fmt.Errorf("add specific IP and port allocations to this node before creating servers")
	}
	used := map[string]bool{}
	var existing []models.Server
	if err := c.service.Servers.FindAll(ctx, bson.M{"nodeId": node.ID}, &existing); err != nil {
		return nil, "", err
	}
	for _, server := range existing {
		if server.Status == "error" {
			continue
		}
		for _, allocation := range server.Allocations {
			used[fmt.Sprintf("%s:%d/*", allocation.IP, allocation.Host)] = true
		}
	}
	allocations := make([]models.PortAllocation, 0, len(ports))
	for _, spec := range ports {
		containerPort := spec.Container
		protocol := strings.ToLower(strings.TrimSpace(spec.Protocol))
		if protocol == "" {
			protocol = "tcp"
		}
		name := spec.Name
		allocated := false
		preferred := 0
		candidates := append([]models.NodePortAllocation(nil), available...)
		if preferred > 0 {
			for i, entry := range candidates {
				if entry.Port == preferred {
					candidates = append([]models.NodePortAllocation{entry}, append(candidates[:i], candidates[i+1:]...)...)
					break
				}
			}
		}
		for _, entry := range candidates {
			if entry.Port < 1 || entry.Port > 65535 || net.ParseIP(entry.IP) == nil {
				continue
			}
			key := fmt.Sprintf("%s:%d/%s", entry.IP, entry.Port, protocol)
			if used[key] || used[fmt.Sprintf("%s:%d/*", entry.IP, entry.Port)] {
				continue
			}
			allocations = append(allocations, models.PortAllocation{Name: name, IP: entry.IP, Host: entry.Port, Container: containerPort, Protocol: protocol})
			used[key] = true
			allocated = true
			break
		}
		if !allocated {
			return nil, "", fmt.Errorf("node has no free specifically allocated ports for all Config ports")
		}
	}
	if len(allocations) > 0 {
		return allocations, allocations[0].IP, nil
	}
	return allocations, ips[0], nil
}
func numberValue(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}

func normalizedServerName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func duplicateServerNameFilter(name string) bson.M {
	return bson.M{"$or": bson.A{
		bson.M{"nameKey": normalizedServerName(name)},
		bson.M{"name": bson.M{"$regex": "^" + regexp.QuoteMeta(strings.TrimSpace(name)) + "$", "$options": "i"}},
	}}
}

func (c *Controller) servers(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		if !requirePermission(w, r, "servers.view") {
			return
		}
		out := make([]models.Server, 0)
		if err := c.service.Servers.FindAll(r.Context(), bson.D{}, &out); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if err := c.redactServerVariables(r.Context(), out); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, out)
		return
	}
	if r.Method == "POST" {
		if !requirePermission(w, r, "servers.create") {
			return
		}
		actor, _ := r.Context().Value(userKey).(models.User)
		unlock := c.lockUser(actor.ID)
		defer unlock()
		var currentActor models.User
		if err := c.service.Users.FindOne(r.Context(), bson.M{"_id": actor.ID, "disabled": bson.M{"$ne": true}}).Decode(&currentActor); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "user account is unavailable"})
			return
		}
		var in struct {
			Name          string            `json:"name"`
			NodeID        bson.ObjectID     `json:"nodeId"`
			ConfigID      bson.ObjectID     `json:"configId"`
			CPULimit      float64           `json:"cpuLimit"`
			MemoryMB      int64             `json:"memoryMB"`
			DiskSizeBytes int64             `json:"diskSizeBytes"`
			Variables     map[string]string `json:"variables"`
		}
		if bodyJSON(r, &in) != nil || strings.TrimSpace(in.Name) == "" || in.NodeID.IsZero() {
			writeJSON(w, 400, map[string]string{"error": "name, nodeId and configId required"})
			return
		}
		if in.ConfigID.IsZero() {
			writeJSON(w, 400, map[string]string{"error": "name, nodeId and configId required"})
			return
		}
		in.Name = strings.TrimSpace(in.Name)
		var duplicate models.Server
		if err := c.service.Servers.FindOne(r.Context(), duplicateServerNameFilter(in.Name)).Decode(&duplicate); err == nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "A server with this name already exists."})
			return
		} else if !errors.Is(err, service.ErrNotFound) {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not check server name availability."})
			return
		}
		var node models.Node
		if err := c.service.Nodes.FindOne(r.Context(), bson.M{"_id": in.NodeID}).Decode(&node); err != nil {
			writeJSON(w, 400, map[string]string{"error": "selected node was not found"})
			return
		}
		var value models.Config
		if err := c.service.Configs.FindOne(r.Context(), bson.M{"_id": in.ConfigID}).Decode(&value); err != nil {
			writeJSON(w, 400, map[string]string{"error": "selected Config was not found"})
			return
		}
		if !config.SupportsArchitecture(value.Spec, node.Stats.Architecture) {
			writeJSON(w, 409, map[string]string{"error": fmt.Sprintf("%s requires a %s node; this node is %s", value.Name, strings.Join(value.Spec.SupportedArchitectures, " or "), node.Stats.Architecture)})
			return
		}
		c.app.nodesMu.RLock()
		nodeConn := c.app.nodeSessions[in.NodeID]
		c.app.nodesMu.RUnlock()
		if nodeConn == nil {
			writeJSON(w, 409, map[string]string{"error": "node is offline; start its agent before creating a server"})
			return
		}
		if in.CPULimit <= 0 {
			in.CPULimit = value.Spec.Resources.CPULimit
			if in.CPULimit <= 0 {
				in.CPULimit = 2
			}
		}
		if in.MemoryMB <= 0 {
			in.MemoryMB = value.Spec.Resources.MemoryMB
			if in.MemoryMB <= 0 {
				in.MemoryMB = 4096
			}
		}
		if in.DiskSizeBytes == 0 {
			in.DiskSizeBytes = 4 * 1024 * 1024 * 1024
		}
		if in.DiskSizeBytes < 128*1024*1024 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "server disk size must be at least 128 MB"})
			return
		}
		resolvedVariables, err := config.DefaultValues(value.Spec, in.Variables)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		allocations, address, err := c.allocatePorts(r.Context(), node, value)
		if err != nil {
			writeJSON(w, 409, map[string]string{"error": err.Error()})
			return
		}
		containerConfig := models.Server{ID: bson.NewObjectID(), Name: in.Name, NameKey: normalizedServerName(in.Name), ManagerID: actor.ID, NodeID: in.NodeID, ConfigID: in.ConfigID, Status: "installing", CPULimit: in.CPULimit, MemoryMB: in.MemoryMB, DiskSizeBytes: in.DiskSizeBytes, Variables: resolvedVariables, Address: address, Allocations: allocations, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		payload := BuildServerCreatePayload(containerConfig, value)
		imageName, _ := payload["image"].(string)
		if strings.TrimSpace(imageName) == "" {
			writeJSON(w, 400, map[string]string{"error": "selected Config has no Docker image"})
			return
		}
		if _, e := c.service.Servers.InsertOne(r.Context(), containerConfig); e != nil {
			if c.service.IsDuplicateKey(e) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "A server with this name already exists."})
				return
			}
			writeJSON(w, 500, map[string]string{"error": e.Error()})
			return
		}
		c.recordAudit(r, "server.created", containerConfig.Name)
		response := containerConfig
		response.Variables = publicServerVariables(value, response.Variables)
		writeJSON(w, http.StatusAccepted, response)
		go c.completeServerCreate(containerConfig.ID, payload, nodeConn)
		return
	}
	http.NotFound(w, r)
}

func (c *Controller) completeServerCreate(serverID bson.ObjectID, payload map[string]any, nodeConn *nodeSession) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	result, err := nodeConn.request(ctx, map[string]any{"type": "create_server", "serverId": serverID.Hex(), "payload": payload})
	updatedAt := time.Now()
	if err != nil {
		_, _ = c.service.Servers.UpdateOne(context.Background(), bson.M{"_id": serverID}, bson.M{"$set": bson.M{"status": "error", "error": err.Error(), "updatedAt": updatedAt}})
		return
	}
	containerID, _ := result["containerId"].(string)
	if strings.TrimSpace(containerID) == "" {
		_, _ = c.service.Servers.UpdateOne(context.Background(), bson.M{"_id": serverID}, bson.M{"$set": bson.M{"status": "error", "error": "Node completed installation without returning a container ID.", "updatedAt": updatedAt}})
		return
	}
	_, _ = c.service.Servers.UpdateOne(context.Background(), bson.M{"_id": serverID}, bson.M{"$set": bson.M{"containerId": containerID, "status": "running", "updatedAt": updatedAt}, "$unset": bson.M{"error": ""}})
}
