package service

import (
	"context"
	"errors"
	"time"

	"github.com/example/control-plane/internal/models"
	"github.com/example/control-plane/internal/panel/repository"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var ErrUnauthenticated = errors.New("unauthorized")

var ErrNotFound = repository.ErrNotFound

type Repository = repository.Repository
type Cursor = repository.Cursor
type WriteResult = repository.WriteResult

// Service contains panel use cases and delegates persistence to Repository.
type Service struct {
	repository                                         repository.Repository
	Users, Sessions, SetupEnrollments, LoginChallenges *CollectionService
	Nodes, Configs, Roles, Servers                     *CollectionService
	Backups, AuditLogs, Settings                       *CollectionService
}

// CollectionService exposes persistence operations through the application layer.
type CollectionService struct {
	repository repository.Repository
	collection string
}

// DocumentResult holds a read error until the caller decodes the matching record.
type DocumentResult struct {
	raw bson.Raw
	err error
}

// InsertResult contains the identifier returned after inserting a document.
type InsertResult struct{ InsertedID any }

// NewService constructs application use cases with the supplied repository.
func NewService(repository repository.Repository) *Service {
	collection := func(name string) *CollectionService {
		return &CollectionService{repository: repository, collection: name}
	}
	return &Service{
		repository: repository,
		Users:      collection("users"), Sessions: collection("sessions"),
		SetupEnrollments: collection("setup_enrollments"), LoginChallenges: collection("login_challenges"),
		Nodes: collection("nodes"), Configs: collection("configs"), Roles: collection("roles"),
		Servers: collection("servers"), Backups: collection("backups"), AuditLogs: collection("audit_logs"),
		Settings: collection("settings"),
	}
}

// FindOne loads one document and defers its error until the response is decoded.
func (c *CollectionService) FindOne(ctx context.Context, filter any) DocumentResult {
	var raw bson.Raw
	err := c.repository.FindOne(ctx, c.collection, filter, &raw)
	return DocumentResult{raw: raw, err: err}
}

// Decode copies the loaded document into a caller-provided domain value.
func (r DocumentResult) Decode(destination any) error {
	if r.err != nil {
		return r.err
	}
	return bson.Unmarshal(r.raw, destination)
}

// Err returns the deferred document lookup error without decoding a value.
func (r DocumentResult) Err() error { return r.err }

// Find opens a cursor over matching documents.
func (c *CollectionService) Find(ctx context.Context, filter any, sort ...any) (Cursor, error) {
	var ordering any
	if len(sort) > 0 {
		ordering = sort[0]
	}
	return c.repository.Find(ctx, c.collection, filter, ordering)
}

// FindAll reads matching records and closes the underlying cursor on every path.
func (c *CollectionService) FindAll(ctx context.Context, filter, destination any, sort ...any) error {
	cursor, err := c.Find(ctx, filter, sort...)
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)
	return cursor.All(ctx, destination)
}

// InsertOne persists one document and returns its generated identifier.
func (c *CollectionService) InsertOne(ctx context.Context, document any) (InsertResult, error) {
	id, err := c.repository.InsertOne(ctx, c.collection, document)
	return InsertResult{InsertedID: id}, err
}

// ReplaceOne stores a full document and optionally enables insert-on-miss.
func (c *CollectionService) ReplaceOne(ctx context.Context, filter, replacement any, upsert bool) (WriteResult, error) {
	return c.repository.ReplaceOne(ctx, c.collection, filter, replacement, upsert)
}

// UpdateOne changes one document and reports matched and modified counts.
func (c *CollectionService) UpdateOne(ctx context.Context, filter, update any, upsert ...bool) (WriteResult, error) {
	insertOnMiss := len(upsert) > 0 && upsert[0]
	return c.repository.UpdateOne(ctx, c.collection, filter, update, insertOnMiss)
}

// DeleteOne removes the first matching document.
func (c *CollectionService) DeleteOne(ctx context.Context, filter any) (DeleteResult, error) {
	deleted, err := c.repository.DeleteOne(ctx, c.collection, filter)
	return DeleteResult{DeletedCount: deleted}, err
}

// DeleteMany removes all matching documents.
func (c *CollectionService) DeleteMany(ctx context.Context, filter any) (DeleteResult, error) {
	deleted, err := c.repository.DeleteMany(ctx, c.collection, filter)
	return DeleteResult{DeletedCount: deleted}, err
}

// CountDocuments returns the number of matching documents.
func (c *CollectionService) CountDocuments(ctx context.Context, filter any) (int64, error) {
	return c.repository.Count(ctx, c.collection, filter)
}

// DeleteResult carries a deleted count across the repository boundary.
type DeleteResult struct{ DeletedCount int64 }

// IsDuplicateKey reports whether persistence rejected a unique-index conflict.
func (s *Service) IsDuplicateKey(err error) bool {
	return s.repository.IsDuplicateKey(err)
}

// IsNotFound identifies a missing record without exposing MongoDB errors.
func (s *Service) IsNotFound(err error) bool {
	return errors.Is(err, repository.ErrNotFound)
}

// Authenticate resolves an unexpired session to an enabled user account.
func (s *Service) Authenticate(ctx context.Context, token string, now time.Time) (models.User, error) {
	var session models.Session
	if token == "" || s.Sessions.FindOne(ctx, bson.M{
		"_id": token, "expiresAt": bson.M{"$gt": now},
	}).Decode(&session) != nil {
		return models.User{}, ErrUnauthenticated
	}
	var user models.User
	if s.Users.FindOne(ctx, bson.M{"_id": session.UserID}).Decode(&user) != nil || user.Disabled {
		return models.User{}, ErrUnauthenticated
	}
	user.Permissions = s.PermissionsForUser(ctx, user)
	return user, nil
}

// PermissionsForUser resolves the role's current permission set.
func (s *Service) PermissionsForUser(ctx context.Context, user models.User) []string {
	if user.Role == "root" {
		return AllPermissions()
	}
	var role models.Role
	if err := s.Roles.FindOne(ctx, bson.M{"_id": user.Role}).Decode(&role); err != nil {
		return nil
	}
	return NormalizePermissions(role.Permissions)
}
