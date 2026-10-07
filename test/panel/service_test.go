package panel_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/example/control-plane/internal/models"
	"github.com/example/control-plane/internal/panel/repository"
	panelservice "github.com/example/control-plane/internal/panel/service"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type memoryRepository struct {
	sessions map[string]models.Session
	users    map[bson.ObjectID]models.User
	roles    map[string]models.Role
}

func (r *memoryRepository) FindOne(_ context.Context, collection string, filter, destination any) error {
	query, ok := filter.(bson.M)
	if !ok {
		return repository.ErrNotFound
	}
	var value any
	switch collection {
	case "sessions":
		session, found := r.sessions[query["_id"].(string)]
		if !found {
			return repository.ErrNotFound
		}
		if expiry, ok := query["expiresAt"].(bson.M); ok && !session.ExpiresAt.After(expiry["$gt"].(time.Time)) {
			return repository.ErrNotFound
		}
		value = session
	case "users":
		user, found := r.users[query["_id"].(bson.ObjectID)]
		if !found {
			return repository.ErrNotFound
		}
		value = user
	case "roles":
		role, found := r.roles[query["_id"].(string)]
		if !found {
			return repository.ErrNotFound
		}
		value = role
	default:
		return repository.ErrNotFound
	}
	raw, err := bson.Marshal(value)
	if err != nil {
		return err
	}
	return bson.Unmarshal(raw, destination)
}

func (r *memoryRepository) Find(context.Context, string, any, any) (repository.Cursor, error) {
	return nil, errors.New("unexpected Find call")
}
func (r *memoryRepository) InsertOne(context.Context, string, any) (any, error) {
	return nil, errors.New("unexpected InsertOne call")
}
func (r *memoryRepository) ReplaceOne(context.Context, string, any, any, bool) (repository.WriteResult, error) {
	return repository.WriteResult{}, errors.New("unexpected ReplaceOne call")
}
func (r *memoryRepository) UpdateOne(context.Context, string, any, any, bool) (repository.WriteResult, error) {
	return repository.WriteResult{}, errors.New("unexpected UpdateOne call")
}
func (r *memoryRepository) DeleteOne(context.Context, string, any) (int64, error) {
	return 0, errors.New("unexpected DeleteOne call")
}
func (r *memoryRepository) DeleteMany(context.Context, string, any) (int64, error) {
	return 0, errors.New("unexpected DeleteMany call")
}
func (r *memoryRepository) Count(context.Context, string, any) (int64, error) {
	return 0, errors.New("unexpected Count call")
}
func (r *memoryRepository) IsDuplicateKey(error) bool { return false }

func TestAuthenticateReturnsEnabledUserWithCurrentRolePermissions(t *testing.T) {
	now := time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC)
	userID := bson.NewObjectID()
	repository := &memoryRepository{
		sessions: map[string]models.Session{
			"valid": {ID: "valid", UserID: userID, ExpiresAt: now.Add(time.Hour)},
		},
		users: map[bson.ObjectID]models.User{
			userID: {ID: userID, Username: "operator", Role: "operator"},
		},
		roles: map[string]models.Role{
			"operator": {ID: "operator", Permissions: []string{"servers.view", "unknown.permission", "servers.view"}},
		},
	}

	user, err := panelservice.NewService(repository).Authenticate(context.Background(), "valid", now)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if user.Username != "operator" {
		t.Fatalf("Authenticate() username = %q, want operator", user.Username)
	}
	if len(user.Permissions) != 1 || user.Permissions[0] != "servers.view" {
		t.Fatalf("effective permissions = %#v, want [servers.view]", user.Permissions)
	}
}

func TestAuthenticateRejectsExpiredAndDisabledSessions(t *testing.T) {
	now := time.Now().UTC()
	userID := bson.NewObjectID()
	repository := &memoryRepository{
		sessions: map[string]models.Session{
			"expired":  {ID: "expired", UserID: userID, ExpiresAt: now},
			"disabled": {ID: "disabled", UserID: userID, ExpiresAt: now.Add(time.Hour)},
		},
		users: map[bson.ObjectID]models.User{
			userID: {ID: userID, Disabled: true},
		},
	}
	service := panelservice.NewService(repository)
	for _, token := range []string{"", "expired", "disabled"} {
		if _, err := service.Authenticate(context.Background(), token, now); !errors.Is(err, panelservice.ErrUnauthenticated) {
			t.Errorf("Authenticate(%q) error = %v, want unauthorized", token, err)
		}
	}
}

func TestAuthenticateAlwaysGrantsRootPermissions(t *testing.T) {
	now := time.Now().UTC()
	userID := bson.NewObjectID()
	repository := &memoryRepository{
		sessions: map[string]models.Session{"root": {ID: "root", UserID: userID, ExpiresAt: now.Add(time.Hour)}},
		users:    map[bson.ObjectID]models.User{userID: {ID: userID, Role: "root"}},
	}
	user, err := panelservice.NewService(repository).Authenticate(context.Background(), "root", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(user.Permissions) != len(panelservice.PermissionCatalog()) {
		t.Fatalf("root has %d permissions; want all %d", len(user.Permissions), len(panelservice.PermissionCatalog()))
	}
}
