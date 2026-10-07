package service

import (
	"sort"

	"github.com/example/control-plane/internal/models"
)

// PermissionDefinition describes one permission available for assignment to a role.
type PermissionDefinition struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Group string `json:"group"`
}

var permissionCatalog = []PermissionDefinition{
	{Key: "dashboard.view", Label: "View dashboard", Group: "Dashboard"},
	{Key: "servers.view", Label: "View servers", Group: "Servers"},
	{Key: "servers.create", Label: "Create servers", Group: "Servers"},
	{Key: "servers.update", Label: "Edit server settings", Group: "Servers"},
	{Key: "servers.delete", Label: "Delete servers", Group: "Servers"},
	{Key: "servers.control", Label: "Start, stop, restart, and deploy", Group: "Servers"},
	{Key: "servers.console", Label: "View and use server consoles", Group: "Servers"},
	{Key: "servers.files.view", Label: "Browse and download server files", Group: "Server files"},
	{Key: "servers.files.manage", Label: "Upload, edit, and delete server files", Group: "Server files"},
	{Key: "servers.backups.view", Label: "View server backups", Group: "Backups"},
	{Key: "servers.backups.manage", Label: "Create, restore, and delete backups", Group: "Backups"},
	{Key: "volumes.extend", Label: "Extend server volumes", Group: "Volumes"},
	{Key: "volumes.files.view", Label: "Browse volume files", Group: "Volumes"},
	{Key: "volumes.files.manage", Label: "Manage volume files", Group: "Volumes"},
	{Key: "nodes.view", Label: "View nodes", Group: "Nodes"},
	{Key: "nodes.manage", Label: "Register, edit, and delete nodes", Group: "Nodes"},
	{Key: "configs.view", Label: "View Configs", Group: "Configs"},
	{Key: "configs.manage", Label: "Create and edit Configs", Group: "Configs"},
	{Key: "users.view", Label: "View users", Group: "User access"},
	{Key: "users.manage", Label: "Create users and assign roles", Group: "User access"},
	{Key: "users.delete", Label: "Delete users and their server data", Group: "User access"},
	{Key: "audit.view", Label: "View audit activity", Group: "User access"},
	{Key: "roles.view", Label: "View roles", Group: "Role access"},
	{Key: "roles.create", Label: "Create roles", Group: "Role access"},
	{Key: "roles.update", Label: "Update roles and permissions", Group: "Role access"},
	{Key: "roles.delete", Label: "Delete roles", Group: "Role access"},
}

// PermissionCatalog returns the known role permissions in their display order.
func PermissionCatalog() []PermissionDefinition {
	return append([]PermissionDefinition(nil), permissionCatalog...)
}

// AllPermissions returns every permission key for the immutable root role.
func AllPermissions() []string {
	permissions := make([]string, 0, len(permissionCatalog))
	for _, permission := range permissionCatalog {
		permissions = append(permissions, permission.Key)
	}
	return permissions
}

// NormalizePermissions removes unknown and repeated entries and returns a stable order.
func NormalizePermissions(input []string) []string {
	known := make(map[string]bool, len(permissionCatalog))
	for _, permission := range permissionCatalog {
		known[permission.Key] = true
	}
	set := make(map[string]bool, len(input))
	for _, permission := range input {
		if known[permission] {
			set[permission] = true
		}
	}
	result := make([]string, 0, len(set))
	for permission := range set {
		result = append(result, permission)
	}
	sort.Strings(result)
	return result
}

// UserHasPermission checks one permission, including the root role's implicit grant.
func UserHasPermission(user models.User, permission string) bool {
	if user.Role == "root" {
		return true
	}
	for _, candidate := range user.Permissions {
		if candidate == permission {
			return true
		}
	}
	return false
}

// HasPermissions checks whether the user has every requested permission.
func HasPermissions(user models.User, requested []string) bool {
	if user.Role == "root" {
		return true
	}
	allowed := make(map[string]bool, len(user.Permissions))
	for _, permission := range user.Permissions {
		allowed[permission] = true
	}
	for _, permission := range requested {
		if !allowed[permission] {
			return false
		}
	}
	return true
}
