package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/mail"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/example/control-plane/internal/models"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// users lists managed accounts or creates a user after checking the actor's permissions.
func (c *Controller) users(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		if !requirePermission(w, r, "users.view") {
			return
		}
		out := make([]models.User, 0)
		if err := c.service.Users.FindAll(r.Context(), bson.D{}, &out); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load users"})
			return
		}
		for index := range out {
			out[index].Permissions = c.service.PermissionsForUser(r.Context(), out[index])
			var managed []models.Server
			if err := c.service.Servers.FindAll(r.Context(), bson.M{"managerId": out[index].ID}, &managed); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load managed servers"})
				return
			}
			out[index].ManagedServers = make([]models.ManagedServer, 0, len(managed))
			for _, server := range managed {
				out[index].ManagedServers = append(out[index].ManagedServers, models.ManagedServer{
					ID: server.ID, Name: server.Name, Status: server.Status,
					NodeID: server.NodeID, ConfigID: server.ConfigID,
				})
			}
		}
		writeJSON(w, 200, out)
		return
	}
	if r.Method == "POST" {
		if !requirePermission(w, r, "users.manage") {
			return
		}
		var in struct {
			Username string `json:"username"`
			Email    string `json:"email"`
			Password string `json:"password"`
			Role     string `json:"role"`
		}
		if bodyJSON(r, &in) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user request"})
			return
		}
		if len(in.Password) < 8 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password must be at least 8 characters"})
			return
		}
		if in.Role == "" {
			in.Role = "user"
		}
		if !c.canAssignRole(r, in.Role) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "you cannot assign that role"})
			return
		}
		h, err := hashPassword(in.Password)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "password hashing failed"})
			return
		}
		u := models.User{ID: bson.NewObjectID(), Username: strings.TrimSpace(in.Username), Email: strings.TrimSpace(in.Email), PasswordHash: h, Role: in.Role, CreatedAt: time.Now()}
		if len(u.Username) < 3 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username must be at least 3 characters"})
			return
		}
		parsedEmail, emailErr := mail.ParseAddress(u.Email)
		if emailErr != nil || parsedEmail.Address != u.Email {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "enter a valid email address"})
			return
		}
		_, e := c.service.Users.InsertOne(r.Context(), u)
		if e != nil {
			if c.service.IsDuplicateKey(e) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "that username or email is already in use"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create user"})
			return
		}
		c.recordAudit(r, "user.created", u.Username)
		writeJSON(w, 201, u)
		return
	}
	http.NotFound(w, r)
}

// canAssignRole checks whether the actor can grant the requested role.
func (c *Controller) canAssignRole(r *http.Request, roleID string) bool {
	actor, _ := r.Context().Value(userKey).(models.User)
	if roleID == "root" {
		return actor.Role == "root"
	}
	var role models.Role
	if err := c.service.Roles.FindOne(r.Context(), bson.M{"_id": roleID}).Decode(&role); err != nil {
		return false
	}
	return hasPermissions(actor, role.Permissions)
}

// userByID reads or updates a single account after enforcing role assignment rules.
func (c *Controller) userByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	permission := "users.manage"
	if r.Method == http.MethodDelete {
		permission = "users.delete"
	}
	if !requirePermission(w, r, permission) {
		return
	}
	id, err := bson.ObjectIDFromHex(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/users/"), "/"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user id"})
		return
	}
	var target models.User
	if err := c.service.Users.FindOne(r.Context(), bson.M{"_id": id}).Decode(&target); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	actor, _ := r.Context().Value(userKey).(models.User)
	if r.Method == http.MethodDelete {
		c.deleteUser(w, r, actor, target)
		return
	}
	if target.Role == "root" && actor.Role != "root" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "the root account cannot be edited by another user"})
		return
	}
	var in struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if bodyJSON(r, &in) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user update"})
		return
	}
	update := bson.M{}
	if username := strings.TrimSpace(in.Username); username != "" {
		if len(username) < 3 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username must be at least 3 characters"})
			return
		}
		update["username"] = username
	}
	if email := strings.TrimSpace(in.Email); email != "" {
		parsed, emailErr := mail.ParseAddress(email)
		if emailErr != nil || parsed.Address != email {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "enter a valid email address"})
			return
		}
		update["email"] = email
	}
	if in.Password != "" {
		if len(in.Password) < 8 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password must be at least 8 characters"})
			return
		}
		hash, err := hashPassword(in.Password)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "password hashing failed"})
			return
		}
		update["passwordHash"] = hash
	}
	if in.Role != "" {
		if target.Role == "root" && in.Role != target.Role {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "the root account role cannot be changed"})
			return
		}
		if in.Role != target.Role && !c.canAssignRole(r, in.Role) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "you cannot assign that role"})
			return
		}
		if in.Role != target.Role {
			update["role"] = in.Role
		}
	}
	if len(update) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provide at least one account field to update"})
		return
	}
	if _, err := c.service.Users.UpdateOne(r.Context(), bson.M{"_id": id}, bson.M{"$set": update}); c.service.IsDuplicateKey(err) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "that username or email is already in use"})
		return
	} else if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update user role"})
		return
	}
	if value, ok := update["username"].(string); ok {
		target.Username = value
	}
	if value, ok := update["email"].(string); ok {
		target.Email = value
	}
	if value, ok := update["role"].(string); ok {
		target.Role = value
	}
	target.Permissions = c.service.PermissionsForUser(r.Context(), target)
	c.recordAudit(r, "user.updated", target.Username)
	writeJSON(w, http.StatusOK, target)
}

// lockUser serializes destructive actions against a specific account.
func (c *Controller) lockUser(id bson.ObjectID) func() {
	value, _ := c.app.userLocks.LoadOrStore(id, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	return lock.Unlock
}

// deleteUser removes an account and its server data while preserving the final root account.
func (c *Controller) deleteUser(w http.ResponseWriter, r *http.Request, actor, target models.User) {
	if target.Role == "root" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "the root account cannot be deleted"})
		return
	}
	if actor.ID == target.ID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "you cannot delete your own account"})
		return
	}
	unlock := c.lockUser(target.ID)
	defer unlock()
	if err := c.service.Users.FindOne(r.Context(), bson.M{"_id": target.ID, "role": bson.M{"$ne": "root"}}).Decode(&target); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	var servers []models.Server
	if err := c.service.Servers.FindAll(r.Context(), bson.M{"managerId": target.ID}, &servers); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load the user's servers"})
		return
	}

	serverIDs := make([]bson.ObjectID, 0, len(servers))
	cleanup := make(map[string]*userServerCleanup, len(servers))
	for _, server := range servers {
		if server.Status == "installing" {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "wait for server installation to finish before deleting this user"})
			return
		}
		serverIDs = append(serverIDs, server.ID)
		cleanup[server.ID.Hex()] = &userServerCleanup{
			serverID: server.ID.Hex(), containerID: server.ContainerID, nodeID: server.NodeID,
		}
		for _, volume := range server.Volumes {
			cleanup[server.ID.Hex()].volumeIDs = append(cleanup[server.ID.Hex()].volumeIDs, volume.ID)
		}
	}

	if len(serverIDs) > 0 {
		var backups []models.Backup
		if err := c.service.Backups.FindAll(r.Context(), bson.M{"serverId": bson.M{"$in": serverIDs}}, &backups); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load server backup images"})
			return
		}
		for _, backup := range backups {
			if target := cleanup[backup.ServerID.Hex()]; target != nil && backup.Image != "" {
				target.backupImages = append(target.backupImages, backup.Image)
			}
		}
	}

	nodeSessions := make(map[bson.ObjectID]*nodeSession)
	c.app.nodesMu.RLock()
	for _, target := range cleanup {
		if nodeSessions[target.nodeID] == nil {
			nodeSessions[target.nodeID] = c.app.nodeSessions[target.nodeID]
		}
		if nodeSessions[target.nodeID] == nil {
			c.app.nodesMu.RUnlock()
			writeJSON(w, http.StatusConflict, map[string]string{"error": "all nodes with this user's servers must be online before deleting the user and server data"})
			return
		}
	}
	c.app.nodesMu.RUnlock()
	result, err := c.service.Users.UpdateOne(r.Context(), bson.M{"_id": target.ID, "role": bson.M{"$ne": "root"}}, bson.M{"$set": bson.M{"disabled": true}})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not disable the user before cleanup"})
		return
	}
	if result.MatchedCount == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	serverKeys := make([]string, 0, len(cleanup))
	for serverID := range cleanup {
		serverKeys = append(serverKeys, serverID)
	}
	sort.Strings(serverKeys)
	for _, serverID := range serverKeys {
		item := cleanup[serverID]
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
		_, err := nodeSessions[item.nodeID].request(ctx, map[string]any{
			"type": "server_delete", "serverId": item.serverID, "containerId": item.containerID,
			"payload": map[string]any{"backupImages": item.backupImages, "volumeIds": item.volumeIDs, "pruneUnusedImages": true},
		})
		cancel()
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("could not remove server %s and its data: %v", item.serverID, err)})
			return
		}
	}

	if len(serverIDs) > 0 {
		if _, err := c.service.Backups.DeleteMany(r.Context(), bson.M{"serverId": bson.M{"$in": serverIDs}}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server data was removed, but backup records could not be cleared"})
			return
		}
	}
	if _, err := c.service.Servers.DeleteMany(r.Context(), bson.M{"managerId": target.ID}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server data was removed, but server records could not be cleared"})
		return
	}
	if _, err := c.service.Sessions.DeleteMany(r.Context(), bson.M{"userId": target.ID}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server data was removed, but user sessions could not be revoked"})
		return
	}
	if _, err := c.service.Users.DeleteOne(r.Context(), bson.M{"_id": target.ID, "role": bson.M{"$ne": "root"}}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server data was removed, but the user account could not be deleted"})
		return
	}
	c.recordAudit(r, "user.deleted", target.Username)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type userServerCleanup struct {
	serverID     string
	containerID  string
	nodeID       bson.ObjectID
	backupImages []string
	volumeIDs    []string
}
