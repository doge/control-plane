package controller

import (
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/example/control-plane/internal/models"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var roleIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func (c *Controller) roles(w http.ResponseWriter, r *http.Request) {
	actor, _ := r.Context().Value(userKey).(models.User)
	if r.URL.Path != "/api/roles" {
		c.roleByID(w, r, strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/roles/"), "/"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !userHasPermission(actor, "roles.view") && !userHasPermission(actor, "roles.create") && !userHasPermission(actor, "roles.update") && !userHasPermission(actor, "roles.delete") && !userHasPermission(actor, "users.manage") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "role view permission is required"})
			return
		}
		roles := make([]models.Role, 0)
		if err := c.service.Roles.FindAll(r.Context(), bson.D{}, &roles); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load roles"})
			return
		}
		roles = append(roles, models.Role{ID: "root", Name: "Root", Description: "System role with every permission. This role cannot be changed or deleted.", Permissions: allPermissions()})
		sort.Slice(roles, func(i, j int) bool { return roles[i].Name < roles[j].Name })
		writeJSON(w, http.StatusOK, map[string]any{"roles": roles, "permissions": permissionCatalog})
	case http.MethodPost:
		if !userHasPermission(actor, "roles.create") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "role create permission is required"})
			return
		}
		var in struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Permissions []string `json:"permissions"`
		}
		if bodyJSON(r, &in) != nil || strings.TrimSpace(in.Name) == "" || len(strings.TrimSpace(in.Name)) > 48 || len(strings.TrimSpace(in.Description)) > 180 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role name is required"})
			return
		}
		permissions, ok := validateRolePermissions(in.Permissions, actor)
		if !ok {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "you can only grant permissions your role has"})
			return
		}
		id := roleID(strings.TrimSpace(in.Name))
		if id == "root" || !roleIDPattern.MatchString(id) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role name must produce a valid, non-reserved role id"})
			return
		}
		if c.roleNameExists(r, "", strings.TrimSpace(in.Name)) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "a role with that name already exists"})
			return
		}
		now := time.Now()
		role := models.Role{ID: id, Name: strings.TrimSpace(in.Name), Description: strings.TrimSpace(in.Description), Permissions: permissions, CreatedAt: now, UpdatedAt: now}
		if _, err := c.service.Roles.InsertOne(r.Context(), role); err != nil {
			if c.service.IsDuplicateKey(err) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "a role with that name already exists"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create role"})
			return
		}
		c.recordAudit(r, "role.created", role.Name)
		writeJSON(w, http.StatusCreated, role)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (c *Controller) roleByID(w http.ResponseWriter, r *http.Request, id string) {
	actor, _ := r.Context().Value(userKey).(models.User)
	if id == "permissions" && r.Method == http.MethodGet {
		if !userHasPermission(actor, "roles.view") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "role view permission is required"})
			return
		}
		writeJSON(w, http.StatusOK, permissionCatalog)
		return
	}
	if id == "root" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "the root role is managed by the system and cannot be changed"})
		return
	}
	switch r.Method {
	case http.MethodPut:
		if !userHasPermission(actor, "roles.update") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "role update permission is required"})
			return
		}
		var in struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Permissions []string `json:"permissions"`
		}
		if bodyJSON(r, &in) != nil || strings.TrimSpace(in.Name) == "" || len(strings.TrimSpace(in.Name)) > 48 || len(strings.TrimSpace(in.Description)) > 180 || roleID(in.Name) == "root" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role name is required"})
			return
		}
		var current models.Role
		if err := c.service.Roles.FindOne(r.Context(), bson.M{"_id": id}).Decode(&current); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "role not found"})
			return
		}
		permissions, ok := validateRoleUpdatePermissions(in.Permissions, actor, current.Permissions)
		if !ok {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "you can only grant permissions your role has"})
			return
		}
		if c.roleNameExists(r, id, strings.TrimSpace(in.Name)) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "a role with that name already exists"})
			return
		}
		result, err := c.service.Roles.UpdateOne(r.Context(), bson.M{"_id": id}, bson.M{"$set": bson.M{"name": strings.TrimSpace(in.Name), "description": strings.TrimSpace(in.Description), "permissions": permissions, "updatedAt": time.Now()}})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update role"})
			return
		}
		if result.MatchedCount == 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "role not found"})
			return
		}
		var role models.Role
		if err := c.service.Roles.FindOne(r.Context(), bson.M{"_id": id}).Decode(&role); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load updated role"})
			return
		}
		c.recordAudit(r, "role.updated", role.Name)
		writeJSON(w, http.StatusOK, role)
	case http.MethodDelete:
		if !userHasPermission(actor, "roles.delete") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "role delete permission is required"})
			return
		}
		count, err := c.service.Users.CountDocuments(r.Context(), bson.M{"role": id})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not check role assignments"})
			return
		}
		if count > 0 {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "reassign users before deleting this role"})
			return
		}
		result, err := c.service.Roles.DeleteOne(r.Context(), bson.M{"_id": id})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not delete role"})
			return
		}
		if result.DeletedCount == 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "role not found"})
			return
		}
		c.recordAudit(r, "role.deleted", id)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (c *Controller) roleNameExists(r *http.Request, id, name string) bool {
	filter := bson.M{"name": bson.M{"$regex": "^" + regexp.QuoteMeta(name) + "$", "$options": "i"}}
	if id != "" {
		filter["_id"] = bson.M{"$ne": id}
	}
	return c.service.Roles.FindOne(r.Context(), filter).Err() == nil
}

func validateRolePermissions(input []string, actor models.User) ([]string, bool) {
	permissions := normalizePermissions(input)
	if len(permissions) != len(input) {
		return nil, false
	}
	return permissions, hasPermissions(actor, permissions)
}

func validateRoleUpdatePermissions(input []string, actor models.User, current []string) ([]string, bool) {
	permissions := normalizePermissions(input)
	if len(permissions) != len(input) {
		return nil, false
	}
	allowed := make(map[string]bool, len(current))
	for _, permission := range current {
		allowed[permission] = true
	}
	for _, permission := range permissions {
		if !allowed[permission] && !userHasPermission(actor, permission) {
			return nil, false
		}
	}
	return permissions, true
}

func roleID(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var out strings.Builder
	separator := false
	for _, char := range name {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			out.WriteRune(char)
			separator = false
		} else if out.Len() > 0 && !separator {
			out.WriteByte('-')
			separator = true
		}
	}
	return strings.Trim(out.String(), "-")
}
