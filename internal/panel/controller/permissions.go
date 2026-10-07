package controller

import (
	"context"

	"github.com/example/control-plane/internal/models"
	"github.com/example/control-plane/internal/panel/service"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type PermissionDefinition = service.PermissionDefinition

var permissionCatalog = service.PermissionCatalog()

func allPermissions() []string                     { return service.AllPermissions() }
func normalizePermissions(input []string) []string { return service.NormalizePermissions(input) }
func userHasPermission(user models.User, permission string) bool {
	return service.UserHasPermission(user, permission)
}
func hasPermissions(user models.User, requested []string) bool {
	return service.HasPermissions(user, requested)
}
func (a *App) permissionsForUser(ctx context.Context, user models.User) []string {
	return a.service.PermissionsForUser(ctx, user)
}

func (a *App) seedRoles(ctx context.Context) error {
	count, err := a.db.Collection("roles").CountDocuments(ctx, bson.D{})
	if err != nil || count != 0 {
		return err
	}
	all := allPermissions()
	admin := make([]string, 0, len(all))
	for _, permission := range all {
		if permission != "roles.create" && permission != "roles.update" && permission != "roles.delete" {
			admin = append(admin, permission)
		}
	}
	roles := []any{
		models.Role{ID: "admin", Name: "Administrator", Description: "Manage panel operations and users without changing roles.", Permissions: admin},
		models.Role{ID: "user", Name: "User", Description: "View the dashboard, servers, and Configs.", Permissions: []string{"dashboard.view", "servers.view", "configs.view"}},
	}
	_, err = a.db.Collection("roles").InsertMany(ctx, roles)
	return err
}
