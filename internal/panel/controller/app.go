package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"crypto/rand"
	"encoding/hex"

	"github.com/example/control-plane/internal/panel/repository"
	"github.com/example/control-plane/internal/panel/service"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/crypto/argon2"
)

type Config struct {
	Addr, MongoURI, MongoDB, StaticDir string
	TLSCertFile, TLSKeyFile            string
	SessionTTL                         time.Duration
}
type App struct {
	db            *mongo.Database
	client        *mongo.Client
	service       *service.Service
	cfg           Config
	nodesMu       sync.RWMutex
	nodeSessions  map[bson.ObjectID]*nodeSession
	userLocks     sync.Map
	rateLimitMu   sync.Mutex
	rateLimits    map[string]rateLimitWindow
	httpsRequired atomic.Bool
}

func New(ctx context.Context, cfg Config) (*App, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(cfg.MongoURI))
	if err != nil {
		return nil, err
	}
	if err = client.Ping(ctx, nil); err != nil {
		return nil, err
	}
	database := client.Database(cfg.MongoDB)
	a := &App{db: database, client: client, service: service.NewService(repository.NewMongoRepository(database)), cfg: cfg, nodeSessions: make(map[bson.ObjectID]*nodeSession)}
	var transport struct {
		HTTPSRequired bool `bson:"httpsRequired"`
	}
	if a.db.Collection("settings").FindOne(ctx, bson.M{"_id": "transport"}).Decode(&transport) == nil {
		a.httpsRequired.Store(transport.HTTPSRequired)
	}
	if err := a.ensureIndexes(ctx); err != nil {
		_ = client.Disconnect(ctx)
		return nil, err
	}
	if err := a.seedRoles(ctx); err != nil {
		return nil, err
	}
	if err := a.seedConfigs(ctx); err != nil {
		return nil, err
	}
	return a, nil
}

// NewWithService builds an App around an injected application service.
func NewWithService(cfg Config, service *service.Service) *App {
	return &App{
		service:      service,
		cfg:          cfg,
		nodeSessions: make(map[bson.ObjectID]*nodeSession),
	}
}

func (a *App) Close(ctx context.Context) error { return a.client.Disconnect(ctx) }

// ensureIndexes enforces unique user identities and creates required lookup indexes.
func (a *App) ensureIndexes(ctx context.Context) error {
	identityCollation := &options.Collation{Locale: "en", Strength: 2}
	_, err := a.db.Collection("users").Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "username", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "username", Value: 1}}, Options: options.Index().SetName("username_case_insensitive_unique").SetUnique(true).SetCollation(identityCollation)},
		{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetName("email_case_insensitive_unique").SetUnique(true).SetCollation(identityCollation)},
	})
	if err != nil {
		return err
	}
	_, _ = a.db.Collection("roles").Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "name", Value: 1}}, Options: options.Index().SetUnique(true)})
	_, _ = a.db.Collection("configs").Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "slug", Value: 1}}, Options: options.Index().SetUnique(true)})
	_, err = a.db.Collection("servers").Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "nameKey", Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"nameKey": bson.M{"$type": "string"}})})
	return err
}
func hashPassword(p string) (string, error) {
	salt := make([]byte, 16)
	if _, e := rand.Read(salt); e != nil {
		return "", e
	}
	h := argon2.IDKey([]byte(p), salt, 3, 64*1024, 2, 32)
	return hex.EncodeToString(salt) + "$" + hex.EncodeToString(h), nil
}
func verifyPassword(p, stored string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 2 {
		return false
	}
	salt, e1 := hex.DecodeString(parts[0])
	want, e2 := hex.DecodeString(parts[1])
	if e1 != nil || e2 != nil {
		return false
	}
	got := argon2.IDKey([]byte(p), salt, 3, 64*1024, 2, 32)
	return string(got) == string(want)
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func bodyJSON(r *http.Request, v any) error {
	return json.NewDecoder(http.MaxBytesReader(nil, r.Body, 2<<20)).Decode(v)
}
func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func staticPath(base, path string) string { return filepath.Join(base, filepath.Clean("/"+path)) }
