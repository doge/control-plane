package controller

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/example/control-plane/internal/models"
	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var nodeUpgrader = websocket.Upgrader{CheckOrigin: SameWebSocketOrigin}

const consoleHistoryLimit = 256 * 1024

type consoleClient struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

func (c *consoleClient) write(message any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteJSON(message)
}

// SameWebSocketOrigin validates browser WebSocket origins against the panel host and scheme.
func SameWebSocketOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" { // Node agents authenticate with their bearer token.
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" || !strings.EqualFold(parsed.Host, r.Host) {
		return false
	}
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	return (parsed.Scheme == "https") == secure && (parsed.Scheme == "https" || parsed.Scheme == "http")
}

type nodeSession struct {
	nodeID          bson.ObjectID
	conn            *websocket.Conn
	writeMu         sync.Mutex
	pendingMu       sync.Mutex
	pending         map[string]chan map[string]any
	clientsMu       sync.RWMutex
	clients         map[string]map[*websocket.Conn]*consoleClient
	consoleLogs     map[string][]string
	consoleLogBytes map[string]int
}

func newNodeSession(nodeID bson.ObjectID, conn *websocket.Conn) *nodeSession {
	return &nodeSession{
		nodeID: nodeID, conn: conn, pending: make(map[string]chan map[string]any),
		clients:     make(map[string]map[*websocket.Conn]*consoleClient),
		consoleLogs: make(map[string][]string), consoleLogBytes: make(map[string]int),
	}
}

func (n *nodeSession) send(message any) error {
	n.writeMu.Lock()
	defer n.writeMu.Unlock()
	writeTimeout := 10 * time.Second
	if request, ok := message.(map[string]any); ok {
		if kind, _ := request["type"].(string); strings.HasPrefix(kind, "file_upload_") {
			writeTimeout = 30 * time.Minute
		}
	}
	if err := n.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return err
	}
	return n.conn.WriteJSON(message)
}

func (n *nodeSession) request(ctx context.Context, message map[string]any) (map[string]any, error) {
	requestID := randomToken(16)
	response := make(chan map[string]any, 1)
	n.pendingMu.Lock()
	n.pending[requestID] = response
	n.pendingMu.Unlock()
	defer func() {
		n.pendingMu.Lock()
		delete(n.pending, requestID)
		n.pendingMu.Unlock()
	}()
	message["requestId"] = requestID
	if err := n.send(message); err != nil {
		return nil, err
	}
	select {
	case result := <-response:
		if payload, ok := result["payload"].(map[string]any); ok {
			if errText, _ := payload["error"].(string); errText != "" {
				return nil, errors.New(errText)
			}
		}
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (n *nodeSession) addConsoleClient(serverID string, conn *websocket.Conn) *consoleClient {
	n.clientsMu.Lock()
	defer n.clientsMu.Unlock()
	client := &consoleClient{conn: conn}
	history := strings.Join(n.consoleLogs[serverID], "")
	_ = client.write(map[string]any{"type": "console_history", "payload": map[string]string{"text": history}})
	if n.clients[serverID] == nil {
		n.clients[serverID] = make(map[*websocket.Conn]*consoleClient)
	}
	n.clients[serverID][conn] = client
	return client
}

func (n *nodeSession) removeConsoleClient(serverID string, conn *websocket.Conn) bool {
	n.clientsMu.Lock()
	defer n.clientsMu.Unlock()
	clients := n.clients[serverID]
	delete(clients, conn)
	if len(clients) == 0 {
		delete(n.clients, serverID)
		delete(n.consoleLogs, serverID)
		delete(n.consoleLogBytes, serverID)
		return true
	}
	return false
}

func (n *nodeSession) publishConsole(serverID string, message map[string]any) {
	n.clientsMu.Lock()
	if messageType, _ := message["type"].(string); messageType == "console_output" {
		if payload, ok := message["payload"].(map[string]any); ok {
			if text, ok := payload["text"].(string); ok && text != "" {
				n.rememberConsoleOutput(serverID, text)
			}
		}
	}
	clients := make([]*consoleClient, 0, len(n.clients[serverID]))
	for _, client := range n.clients[serverID] {
		clients = append(clients, client)
	}
	n.clientsMu.Unlock()
	for _, client := range clients {
		_ = client.write(message)
	}
}

// rememberConsoleOutput keeps a bounded replay window for browsers joining an active console.
func (n *nodeSession) rememberConsoleOutput(serverID, text string) {
	if len(text) > consoleHistoryLimit {
		text = text[len(text)-consoleHistoryLimit:]
		n.consoleLogs[serverID] = nil
		n.consoleLogBytes[serverID] = 0
	}
	n.consoleLogs[serverID] = append(n.consoleLogs[serverID], text)
	n.consoleLogBytes[serverID] += len(text)
	for n.consoleLogBytes[serverID] > consoleHistoryLimit && len(n.consoleLogs[serverID]) > 1 {
		first := n.consoleLogs[serverID][0]
		n.consoleLogs[serverID] = n.consoleLogs[serverID][1:]
		n.consoleLogBytes[serverID] -= len(first)
	}
}

func (a *App) syncManagedServerStatuses(ctx context.Context, nodeID bson.ObjectID, raw any) {
	text := func(value any) string {
		result, _ := value.(string)
		return result
	}
	statuses, ok := raw.([]any)
	if !ok {
		return
	}
	for _, rawStatus := range statuses {
		item, ok := rawStatus.(map[string]any)
		if !ok {
			continue
		}
		serverID, err := bson.ObjectIDFromHex(strings.TrimSpace(text(item["serverId"])))
		if err != nil {
			continue
		}
		status := text(item["status"])
		if status != "running" && status != "stopped" && status != "paused" {
			continue
		}
		filter := bson.M{"_id": serverID, "nodeId": nodeID}
		set := bson.M{"status": status, "updatedAt": time.Now()}
		if containerID := strings.TrimSpace(text(item["containerId"])); containerID != "" {
			set["containerId"] = containerID
			filter["$or"] = bson.A{
				bson.M{"status": bson.M{"$ne": status}},
				bson.M{"containerId": bson.M{"$ne": containerID}},
			}
		} else {
			filter["status"] = bson.M{"$ne": status}
		}
		_, _ = a.db.Collection("servers").UpdateOne(ctx, filter, bson.M{"$set": set})
	}
}

func (a *App) nodeConnect(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	nodes := make([]models.Node, 0)
	cur, err := a.db.Collection("nodes").Find(r.Context(), bson.D{})
	if err != nil {
		http.Error(w, "could not look up node", http.StatusInternalServerError)
		return
	}
	_ = cur.All(r.Context(), &nodes)
	_ = cur.Close(r.Context())
	var node *models.Node
	for i := range nodes {
		if verifyPassword(token, nodes[i].TokenHash) {
			node = &nodes[i]
			break
		}
	}
	if node == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := nodeUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
	session := newNodeSession(node.ID, conn)
	a.nodesMu.Lock()
	previous := a.nodeSessions[node.ID]
	a.nodeSessions[node.ID] = session
	a.nodesMu.Unlock()
	if previous != nil {
		_ = previous.conn.Close()
	}
	_, _ = a.db.Collection("nodes").UpdateOne(r.Context(), bson.M{"_id": node.ID}, bson.M{"$set": bson.M{"status": "online", "lastSeen": time.Now()}})
	defer func() {
		a.nodesMu.Lock()
		if a.nodeSessions[node.ID] == session {
			delete(a.nodeSessions, node.ID)
			_, _ = a.db.Collection("nodes").UpdateOne(context.Background(), bson.M{"_id": node.ID}, bson.M{"$set": bson.M{"status": "offline", "lastSeen": time.Now()}})
		}
		a.nodesMu.Unlock()
		_ = conn.Close()
	}()
	heartbeatCtx, stopHeartbeat := context.WithCancel(r.Context())
	defer stopHeartbeat()
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
				if err := session.send(map[string]any{"type": "ping"}); err != nil {
					_ = conn.Close()
					return
				}
			}
		}
	}()
	for {
		var message map[string]any
		if err := conn.ReadJSON(&message); err != nil {
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
		if message["type"] == "hello" || message["type"] == "pong" {
			set := bson.M{"status": "online", "lastSeen": time.Now()}
			if message["type"] == "hello" {
				if payload, ok := message["payload"].(map[string]any); ok {
					if architecture, ok := payload["architecture"].(string); ok && architecture != "" {
						set["stats.architecture"] = architecture
						set["stats.updatedAt"] = time.Now()
					}
					if storageMode, ok := payload["storageMode"].(string); ok && storageMode != "" {
						set["stats.storageMode"] = storageMode
					}
					if bindIPs, ok := payload["bindIPs"].([]any); ok {
						detected := parseNodeBindIPs(bindIPs)
						set["stats.bindIPs"] = detected
						a.reconcileNodeBindAllocations(r.Context(), *node, detected)
					}
					a.syncManagedServerStatuses(r.Context(), node.ID, payload["serverStatuses"])
				}
			}
			_, _ = a.db.Collection("nodes").UpdateOne(r.Context(), bson.M{"_id": node.ID}, bson.M{"$set": set})
		}
		if message["type"] == "node_stats" {
			if stats, ok := message["payload"].(map[string]any); ok {
				stats["updatedAt"] = time.Now()
				_, _ = a.db.Collection("nodes").UpdateOne(r.Context(), bson.M{"_id": node.ID}, bson.M{"$set": bson.M{"stats": stats, "lastSeen": time.Now(), "status": "online"}})
			}
		}
		if message["type"] == "server_statuses" {
			if payload, ok := message["payload"].(map[string]any); ok {
				a.syncManagedServerStatuses(r.Context(), node.ID, payload["serverStatuses"])
			}
		}
		if requestID, ok := message["requestId"].(string); ok {
			session.pendingMu.Lock()
			response := session.pending[requestID]
			session.pendingMu.Unlock()
			if response != nil {
				response <- message
			}
		}
		if message["type"] == "console_output" || message["type"] == "console_error" || message["type"] == "console_attached" {
			serverID, _ := message["serverId"].(string)
			session.publishConsole(serverID, message)
		}
	}
}

// parseNodeBindIPs keeps only valid IP strings reported by an authenticated node agent.
func parseNodeBindIPs(values []any) []string {
	addresses := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			continue
		}
		ip := net.ParseIP(strings.TrimSpace(text))
		if ip == nil {
			continue
		}
		canonical := ip.String()
		if !seen[canonical] {
			addresses = append(addresses, canonical)
			seen[canonical] = true
		}
	}
	return addresses
}

func (a *App) serverConsole(w http.ResponseWriter, r *http.Request) {
	part := strings.TrimPrefix(r.URL.Path, "/api/servers/")
	if !strings.HasSuffix(part, "/console") {
		http.NotFound(w, r)
		return
	}
	idText := strings.TrimSuffix(part, "/console")
	idText = strings.TrimSuffix(idText, "/")
	serverID, err := bson.ObjectIDFromHex(idText)
	if err != nil {
		http.Error(w, "invalid server id", http.StatusBadRequest)
		return
	}
	var server models.Server
	if err := a.db.Collection("servers").FindOne(r.Context(), bson.M{"_id": serverID}).Decode(&server); err != nil {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}
	if server.Status == "installing" {
		http.Error(w, "server is installing", http.StatusConflict)
		return
	}
	if server.Status != "running" {
		http.Error(w, "server is not running", http.StatusConflict)
		return
	}
	if server.ContainerID == "" {
		http.Error(w, "server has no container id; deploy it first", http.StatusConflict)
		return
	}
	a.nodesMu.RLock()
	node := a.nodeSessions[server.NodeID]
	a.nodesMu.RUnlock()
	if node == nil {
		http.Error(w, "node is offline", http.StatusConflict)
		return
	}
	client, err := nodeUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	serverTextID := server.ID.Hex()
	consoleClient := node.addConsoleClient(serverTextID, client)
	// Always ask the agent to validate or refresh the stream. This covers a
	// reconnect racing with the previous websocket's close notification.
	if err := node.send(map[string]any{"type": "console_attach", "serverId": serverTextID, "containerId": server.ContainerID}); err != nil {
		_ = consoleClient.write(map[string]any{"type": "console_error", "text": "Could not attach to node console"})
		_ = client.Close()
		return
	}
	_ = consoleClient.write(map[string]any{"type": "console_status", "text": "Connecting to server console\n"})
	defer func() {
		last := node.removeConsoleClient(serverTextID, client)
		if last {
			_ = node.send(map[string]any{"type": "console_detach", "serverId": serverTextID})
		}
		_ = client.Close()
	}()
	for {
		var input map[string]any
		if err := client.ReadJSON(&input); err != nil {
			return
		}
		command, _ := input["command"].(string)
		if strings.TrimSpace(command) == "" {
			continue
		}
		if err := node.send(map[string]any{"type": "console_input", "serverId": serverTextID, "containerId": server.ContainerID, "command": command}); err != nil {
			_ = consoleClient.write(map[string]any{"type": "console_error", "text": "Node connection was lost"})
			return
		}
	}
}
