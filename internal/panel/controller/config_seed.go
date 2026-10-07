package controller

import (
	"context"
	"fmt"
	"time"

	configfiles "github.com/example/control-plane/configs"
	"github.com/example/control-plane/internal/config"
	"github.com/example/control-plane/internal/models"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const builtinConfigAuthor = "Control Plane"

func (a *App) seedConfigs(ctx context.Context) error {
	defaults, err := configfiles.Defaults()
	if err != nil {
		return err
	}
	for _, value := range defaults {
		if err := config.Validate(value); err != nil {
			return fmt.Errorf("invalid built-in config %q: %w", value.Slug, err)
		}
		var current models.Config
		err := a.db.Collection("configs").FindOne(ctx, bson.M{"slug": value.Slug}).Decode(&current)
		if err == nil {
			continue
		}
		if err != mongo.ErrNoDocuments {
			return err
		}
		value.ID = bson.NewObjectID()
		value.Author = builtinConfigAuthor
		value.CreatedAt = time.Now()
		value.UpdatedAt = value.CreatedAt
		if _, err := a.db.Collection("configs").InsertOne(ctx, value); err != nil {
			return err
		}
	}
	return nil
}
