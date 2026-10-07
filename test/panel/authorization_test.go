package panel_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/control-plane/internal/models"
	"github.com/example/control-plane/internal/panel/controller"
	panelservice "github.com/example/control-plane/internal/panel/service"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func appWithPermissions(role string, permissions []string) (*controller.App, string) {
	now := time.Now().UTC()
	userID := bson.NewObjectID()
	token := "test-session"
	repository := &memoryRepository{
		sessions: map[string]models.Session{
			token: {ID: token, UserID: userID, ExpiresAt: now.Add(time.Hour)},
		},
		users: map[bson.ObjectID]models.User{
			userID: {ID: userID, Username: "tester", Role: role},
		},
		roles: map[string]models.Role{
			role: {ID: role, Permissions: permissions},
		},
	}
	service := panelservice.NewService(repository)
	return controller.NewWithService(controller.Config{}, service), token
}

func serveAs(app *controller.App, token, method, path string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "http://panel.example"+path, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	return response
}

func TestRootAlwaysHasEveryPermission(t *testing.T) {
	for _, permission := range panelservice.AllPermissions() {
		if !panelservice.UserHasPermission(models.User{Role: "root"}, permission) {
			t.Errorf("root is missing %q", permission)
		}
	}
}

func TestServerFilesRequirePermissionBeforeLookingUpServer(t *testing.T) {
	app, token := appWithPermissions("operator", []string{"servers.view"})
	response := serveAs(app, token, http.MethodGet,
		"/api/servers/"+bson.NewObjectID().Hex()+"/files")
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestUserDeleteRequiresDeletePermission(t *testing.T) {
	app, token := appWithPermissions("operator", []string{"users.manage"})
	response := serveAs(app, token, http.MethodDelete,
		"/api/admin/users/"+bson.NewObjectID().Hex())
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestNodeResourcesRequireNodeView(t *testing.T) {
	app, token := appWithPermissions("operator", []string{"nodes.manage"})
	response := serveAs(app, token, http.MethodGet,
		"/api/nodes/"+bson.NewObjectID().Hex()+"/resources")
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestTransportSettingsAreRootOnly(t *testing.T) {
	app, token := appWithPermissions("operator", []string{"users.manage"})
	response := serveAs(app, token, http.MethodGet, "/api/settings/transport")
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestWebSocketOriginMustMatchPanelHostAndScheme(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://panel.example/api/servers/x/console", nil)
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("Origin", "https://panel.example")
	if !controller.SameWebSocketOrigin(request) {
		t.Fatal("same-origin HTTPS websocket was rejected")
	}
	request.Header.Set("Origin", "https://attacker.example")
	if controller.SameWebSocketOrigin(request) {
		t.Fatal("cross-origin websocket was accepted")
	}
	request.Header.Set("Origin", "http://panel.example")
	if controller.SameWebSocketOrigin(request) {
		t.Fatal("insecure origin was accepted for an HTTPS panel")
	}
	request.Header.Del("Origin")
	if !controller.SameWebSocketOrigin(request) {
		t.Fatal("node agent websocket without Origin was rejected")
	}
}
