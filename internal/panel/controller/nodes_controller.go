package controller

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/example/control-plane/internal/models"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// nodes lists the nodes visible to the caller or enrolls a node.
func (c *Controller) nodes(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		if !requirePermission(w, r, "nodes.view") {
			return
		}
		out := make([]models.Node, 0)
		if err := c.service.Nodes.FindAll(r.Context(), bson.D{}, &out); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		for i := range out {
			c.hydrateNodeNetwork(r.Context(), &out[i])
		}
		writeJSON(w, 200, out)
		return
	}
	if r.Method == "POST" {
		if !requirePermission(w, r, "nodes.manage") {
			return
		}
		var in struct {
			Name    string   `json:"name"`
			Region  string   `json:"region"`
			Address string   `json:"address"`
			IPs     []string `json:"ips"`
			Ports   []int    `json:"ports"`
		}
		if bodyJSON(r, &in) != nil || strings.TrimSpace(in.Name) == "" {
			writeJSON(w, 400, map[string]string{"error": "name required"})
			return
		}
		address, err := resolveHost(in.Address)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": "address must be an IP or a DNS name that resolves to an IP"})
			return
		}
		ips, err := resolveAllIPs(address)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": "address hostname must resolve to at least one valid IP"})
			return
		}
		extraIPs, err := validateIPAddresses(in.IPs)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		ips = mergeIPs(ips, extraIPs)
		if len(in.Ports) == 0 {
			in.Ports = []int{25565}
		}
		portsSeen := map[int]bool{}
		for _, port := range in.Ports {
			if port < 1 || port > 65535 || portsSeen[port] {
				writeJSON(w, 400, map[string]string{"error": "node ports must be unique numbers from 1 to 65535"})
				return
			}
			portsSeen[port] = true
		}
		raw := randomToken(24)
		h, _ := hashPassword(raw)
		allocations := make([]models.NodePortAllocation, 0, len(ips)*len(in.Ports))
		for _, ip := range ips {
			for _, port := range in.Ports {
				allocations = append(allocations, models.NodePortAllocation{IP: ip, Port: port})
			}
		}
		n := models.Node{ID: bson.NewObjectID(), Name: strings.TrimSpace(in.Name), Region: in.Region, Address: address, IPs: ips, PortAllocations: allocations, TokenHash: h, Status: "pending", CreatedAt: time.Now()}
		_, e := c.service.Nodes.InsertOne(r.Context(), n)
		if e != nil {
			writeJSON(w, 500, map[string]string{"error": e.Error()})
			return
		}
		c.recordAudit(r, "node.created", n.Name)
		writeJSON(w, 201, map[string]any{"node": n, "token": raw})
		return
	}
	http.NotFound(w, r)
}

// nodeByID reads or updates one node, including its allocation configuration.
func (c *Controller) nodeByID(w http.ResponseWriter, r *http.Request) {
	part := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/nodes/"), "/")
	if part == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(part, "/")
	id, err := bson.ObjectIDFromHex(parts[0])
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid node id"})
		return
	}
	if len(parts) == 2 && parts[1] == "resources" {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if !requirePermission(w, r, "nodes.view") {
			return
		}
		c.app.nodesMu.RLock()
		nodeConn := c.app.nodeSessions[id]
		c.app.nodesMu.RUnlock()
		if nodeConn == nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "node must be online to inspect its Docker resources"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		result, err := nodeConn.request(ctx, map[string]any{"type": "node_resources"})
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not load node Docker resources"})
			return
		}
		resources, _ := result["payload"].(map[string]any)
		if containers, ok := resources["containers"].([]any); ok {
			for _, rawContainer := range containers {
				item, ok := rawContainer.(map[string]any)
				if !ok {
					continue
				}
				filter := bson.M{"nodeId": id}
				if serverID, ok := item["serverId"].(string); ok && serverID != "" {
					if objectID, parseErr := bson.ObjectIDFromHex(serverID); parseErr == nil {
						filter["_id"] = objectID
					}
				}
				if _, ok := filter["_id"]; !ok {
					filter["containerId"] = item["id"]
				}
				var server models.Server
				if err := c.service.Servers.FindOne(ctx, filter).Decode(&server); err == nil {
					item["serverId"] = server.ID.Hex()
					item["serverName"] = server.Name
					item["serverStatus"] = server.Status
					if !server.ManagerID.IsZero() {
						var owner models.User
						if err := c.service.Users.FindOne(ctx, bson.M{"_id": server.ManagerID}).Decode(&owner); err == nil {
							item["ownerName"] = owner.Username
						}
					}
				}
			}
		}
		writeJSON(w, http.StatusOK, resources)
		return
	}
	if len(parts) == 3 && (parts[1] == "containers" || parts[1] == "images") {
		if r.Method != http.MethodDelete {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if !requirePermission(w, r, "nodes.manage") {
			return
		}
		resourceID := strings.TrimSpace(parts[2])
		if resourceID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "resource ID is required"})
			return
		}
		c.app.nodesMu.RLock()
		nodeConn := c.app.nodeSessions[id]
		c.app.nodesMu.RUnlock()
		if nodeConn == nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "node must be online to manage its Docker resources"})
			return
		}
		operation := "node_container_delete"
		key := "containerId"
		if parts[1] == "images" {
			operation = "node_image_delete"
			key = "imageId"
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		result, err := nodeConn.request(ctx, map[string]any{
			"type":    operation,
			"payload": map[string]any{key: resourceID},
		})
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not remove Docker resource: " + err.Error()})
			return
		}
		if parts[1] == "containers" {
			_, err = c.service.Servers.UpdateOne(ctx,
				bson.M{"nodeId": id, "containerId": resourceID},
				bson.M{"$set": bson.M{"containerId": "", "status": "stopped", "updatedAt": time.Now()}},
			)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "container was removed but its server status could not be updated"})
				return
			}
		}
		writeJSON(w, http.StatusOK, result["payload"])
		return
	}
	if len(parts) != 1 {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodGet {
		if !requirePermission(w, r, "nodes.view") {
			return
		}
		var node models.Node
		if err := c.service.Nodes.FindOne(r.Context(), bson.M{"_id": id}).Decode(&node); err != nil {
			writeJSON(w, 404, map[string]string{"error": "node not found"})
			return
		}
		writeJSON(w, 200, node)
		return
	}
	if r.Method == http.MethodDelete {
		if !requirePermission(w, r, "nodes.manage") {
			return
		}
		c.deleteNode(w, r, id)
		return
	}
	if r.Method != "PUT" {
		http.NotFound(w, r)
		return
	}
	if !requirePermission(w, r, "nodes.manage") {
		return
	}
	var in struct {
		Address         string                      `json:"address"`
		IPs             []string                    `json:"ips"`
		PortAllocations []models.NodePortAllocation `json:"portAllocations"`
	}
	if bodyJSON(r, &in) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid node configuration"})
		return
	}
	address, err := resolveHost(in.Address)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "address must be an IP or a DNS name that resolves to an IP"})
		return
	}
	ips, err := resolveAllIPs(address)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "address hostname must resolve to at least one valid IP"})
		return
	}
	extraIPs, err := validateIPAddresses(in.IPs)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	ips = mergeIPs(ips, extraIPs)
	knownIPs := map[string]bool{}
	for _, ip := range ips {
		knownIPs[ip] = true
	}
	allocationKeys := map[string]bool{}
	for _, allocation := range in.PortAllocations {
		if net.ParseIP(allocation.IP) == nil || allocation.Port < 1 || allocation.Port > 65535 {
			writeJSON(w, 400, map[string]string{"error": "each allocation needs a valid node IP and port number"})
			return
		}
		if !knownIPs[allocation.IP] {
			writeJSON(w, 400, map[string]string{"error": "allocation IP must be returned by the hostname or listed as an additional IP"})
			return
		}
		key := fmt.Sprintf("%s:%d", allocation.IP, allocation.Port)
		if allocationKeys[key] {
			writeJSON(w, 400, map[string]string{"error": "node port allocations must be unique per IP"})
			return
		}
		allocationKeys[key] = true
	}
	_, err = c.service.Nodes.UpdateOne(r.Context(), bson.M{"_id": id}, bson.M{"$set": bson.M{"address": address, "ips": ips, "portAllocations": in.PortAllocations}, "$unset": bson.M{"ipPortPools": "", "portStart": "", "portEnd": ""}})
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	ports := make([]int, 0, len(in.PortAllocations))
	seenPorts := make(map[int]bool, len(in.PortAllocations))
	for _, allocation := range in.PortAllocations {
		if !seenPorts[allocation.Port] {
			ports = append(ports, allocation.Port)
			seenPorts[allocation.Port] = true
		}
	}
	c.app.nodesMu.RLock()
	nodeConn := c.app.nodeSessions[id]
	c.app.nodesMu.RUnlock()
	if nodeConn != nil && len(ports) > 0 {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		_, firewallErr := nodeConn.request(ctx, map[string]any{
			"type": "node_firewall_allow", "payload": map[string]any{"ports": ports},
		})
		cancel()
		if firewallErr != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{
				"error": "port allocations were saved, but UFW rules could not be applied: " + firewallErr.Error(),
			})
			return
		}
	}
	var node models.Node
	if c.service.Nodes.FindOne(r.Context(), bson.M{"_id": id}).Decode(&node) != nil {
		writeJSON(w, 404, map[string]string{"error": "node not found"})
		return
	}
	writeJSON(w, 200, node)
}

// deleteNode removes a node after confirming it has no dependent servers.
func (c *Controller) deleteNode(w http.ResponseWriter, r *http.Request, id bson.ObjectID) {
	var node models.Node
	if err := c.service.Nodes.FindOne(r.Context(), bson.M{"_id": id}).Decode(&node); err != nil {
		writeJSON(w, 404, map[string]string{"error": "node not found"})
		return
	}

	var servers []models.Server
	if err := c.service.Servers.FindAll(r.Context(), bson.M{"nodeId": id}, &servers); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	var backups []models.Backup
	if err := c.service.Backups.FindAll(r.Context(), bson.M{"nodeId": id}, &backups); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	type cleanupTarget struct {
		containerID  string
		backupImages []any
		volumeIDs    []string
	}
	targets := make(map[string]*cleanupTarget, len(servers))
	for _, server := range servers {
		volumeIDs := make([]string, 0, len(server.Volumes))
		for _, volume := range server.Volumes {
			volumeIDs = append(volumeIDs, volume.ID)
		}
		targets[server.ID.Hex()] = &cleanupTarget{containerID: server.ContainerID, volumeIDs: volumeIDs}
	}
	for _, backup := range backups {
		serverID := backup.ServerID.Hex()
		target := targets[serverID]
		if target == nil {
			target = &cleanupTarget{}
			targets[serverID] = target
		}
		if backup.Image != "" {
			target.backupImages = append(target.backupImages, backup.Image)
		}
	}

	c.app.nodesMu.RLock()
	nodeConn := c.app.nodeSessions[id]
	c.app.nodesMu.RUnlock()
	if len(targets) > 0 && nodeConn == nil {
		writeJSON(w, 409, map[string]string{"error": "node must be online to remove its server containers and backups; reconnect the agent and retry"})
		return
	}
	if nodeConn != nil {
		serverIDs := make([]string, 0, len(targets))
		for serverID := range targets {
			serverIDs = append(serverIDs, serverID)
		}
		sort.Strings(serverIDs)
		for _, serverID := range serverIDs {
			target := targets[serverID]
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
			_, requestErr := nodeConn.request(ctx, map[string]any{
				"type": "server_delete", "serverId": serverID, "containerId": target.containerID,
				"payload": map[string]any{"backupImages": target.backupImages, "volumeIds": target.volumeIDs},
			})
			cancel()
			if requestErr != nil {
				writeJSON(w, 502, map[string]string{"error": fmt.Sprintf("could not remove server %s from node: %v", serverID, requestErr)})
				return
			}
		}
	}

	if _, err := c.service.Backups.DeleteMany(r.Context(), bson.M{"nodeId": id}); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if _, err := c.service.Servers.DeleteMany(r.Context(), bson.M{"nodeId": id}); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if _, err := c.service.Nodes.DeleteOne(r.Context(), bson.M{"_id": id}); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	c.app.nodesMu.Lock()
	if c.app.nodeSessions[id] == nodeConn {
		delete(c.app.nodeSessions, id)
	}
	c.app.nodesMu.Unlock()
	if nodeConn != nil {
		_ = nodeConn.conn.Close()
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
