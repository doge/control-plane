package repository

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var ErrNotFound = errors.New("document not found")

// Repository defines the persistence operations used by the panel services.
type Repository interface {
	FindOne(context.Context, string, any, any) error
	Find(context.Context, string, any, any) (Cursor, error)
	InsertOne(context.Context, string, any) (any, error)
	ReplaceOne(context.Context, string, any, any, bool) (WriteResult, error)
	UpdateOne(context.Context, string, any, any, bool) (WriteResult, error)
	DeleteOne(context.Context, string, any) (int64, error)
	DeleteMany(context.Context, string, any) (int64, error)
	Count(context.Context, string, any) (int64, error)
	IsDuplicateKey(error) bool
}

// Cursor is the small read interface services need for result iteration.
type Cursor interface {
	All(context.Context, any) error
	Close(context.Context) error
}

// WriteResult exposes the write counts services need without leaking MongoDB types.
type WriteResult struct {
	MatchedCount  int64
	ModifiedCount int64
	DeletedCount  int64
}

// MongoRepository adapts MongoDB collections to the persistence interface.
type MongoRepository struct {
	database *mongo.Database
}

// NewMongoRepository creates a repository backed by one panel database.
func NewMongoRepository(database *mongo.Database) *MongoRepository {
	return &MongoRepository{database: database}
}

// FindOne decodes the first matching document into destination.
func (r *MongoRepository) FindOne(ctx context.Context, collection string, filter any, destination any) error {
	err := r.database.Collection(collection).FindOne(ctx, filter).Decode(destination)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrNotFound
	}
	return err
}

// Find creates a cursor for matching documents without exposing MongoDB types.
func (r *MongoRepository) Find(ctx context.Context, collection string, filter, sort any) (Cursor, error) {
	if sort == nil {
		return r.database.Collection(collection).Find(ctx, filter)
	}
	return r.database.Collection(collection).Find(ctx, filter, options.Find().SetSort(sort))
}

// InsertOne adds a document to the requested collection.
func (r *MongoRepository) InsertOne(ctx context.Context, collection string, document any) (any, error) {
	result, err := r.database.Collection(collection).InsertOne(ctx, document)
	if err != nil {
		return nil, err
	}
	return result.InsertedID, nil
}

// ReplaceOne replaces a matching document and optionally inserts it if absent.
func (r *MongoRepository) ReplaceOne(ctx context.Context, collection string, filter, replacement any, upsert bool) (WriteResult, error) {
	options := options.Replace().SetUpsert(upsert)
	result, err := r.database.Collection(collection).ReplaceOne(ctx, filter, replacement, options)
	if err != nil {
		return WriteResult{}, err
	}
	return WriteResult{MatchedCount: result.MatchedCount}, nil
}

// UpdateOne updates the first matching document and reports affected counts.
func (r *MongoRepository) UpdateOne(ctx context.Context, collection string, filter, update any, upsert bool) (WriteResult, error) {
	var result *mongo.UpdateResult
	var err error
	if upsert {
		result, err = r.database.Collection(collection).UpdateOne(ctx, filter, update, options.UpdateOne().SetUpsert(true))
	} else {
		result, err = r.database.Collection(collection).UpdateOne(ctx, filter, update)
	}
	if err != nil {
		return WriteResult{}, err
	}
	return WriteResult{MatchedCount: result.MatchedCount, ModifiedCount: result.ModifiedCount}, nil
}

// DeleteOne removes the first matching document.
func (r *MongoRepository) DeleteOne(ctx context.Context, collection string, filter any) (int64, error) {
	result, err := r.database.Collection(collection).DeleteOne(ctx, filter)
	if err != nil {
		return 0, err
	}
	return result.DeletedCount, nil
}

// DeleteMany removes all documents matching the filter.
func (r *MongoRepository) DeleteMany(ctx context.Context, collection string, filter any) (int64, error) {
	result, err := r.database.Collection(collection).DeleteMany(ctx, filter)
	if err != nil {
		return 0, err
	}
	return result.DeletedCount, nil
}

// Count returns the number of documents matching a filter.
func (r *MongoRepository) Count(ctx context.Context, collection string, filter any) (int64, error) {
	return r.database.Collection(collection).CountDocuments(ctx, filter)
}

// IsDuplicateKey identifies unique-index conflicts across persistence adapters.
func (r *MongoRepository) IsDuplicateKey(err error) bool {
	return mongo.IsDuplicateKeyError(err)
}

var _ Repository = (*MongoRepository)(nil)
