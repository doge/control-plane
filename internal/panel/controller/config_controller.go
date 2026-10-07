package controller

import (
	"net/http"
	"strings"
	"time"

	"github.com/example/control-plane/internal/config"
	"github.com/example/control-plane/internal/models"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func (c *Controller) configs(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if !requirePermission(w, r, "configs.view") {
			return
		}
		result := make([]models.Config, 0)
		if err := c.service.Configs.FindAll(r.Context(), bson.D{}, &result); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !requirePermission(w, r, "configs.manage") {
		return
	}
	var value models.Config
	if bodyJSON(r, &value) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid Config definition"})
		return
	}
	value.Slug = strings.ToLower(strings.TrimSpace(value.Slug))
	if err := config.Validate(value); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	value.ID = bson.NewObjectID()
	value.CreatedAt = time.Now()
	value.UpdatedAt = value.CreatedAt
	if _, err := c.service.Configs.InsertOne(r.Context(), value); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	c.recordAudit(r, "config.created", value.Name)
	writeJSON(w, http.StatusCreated, value)
}

func (c *Controller) configByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !requirePermission(w, r, "configs.manage") {
		return
	}
	id, err := bson.ObjectIDFromHex(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/configs/"), "/"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid Config id"})
		return
	}
	var value models.Config
	if bodyJSON(r, &value) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid Config definition"})
		return
	}
	value.Slug = strings.ToLower(strings.TrimSpace(value.Slug))
	if err := config.Validate(value); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	value.UpdatedAt = time.Now()
	result, err := c.service.Configs.UpdateOne(r.Context(), bson.M{"_id": id}, bson.M{"$set": bson.M{
		"name": value.Name, "slug": value.Slug, "description": value.Description,
		"author": value.Author, "spec": value.Spec, "updatedAt": value.UpdatedAt,
	}})
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	if result.MatchedCount == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Config not found"})
		return
	}
	if err := c.service.Configs.FindOne(r.Context(), bson.M{"_id": id}).Decode(&value); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	c.recordAudit(r, "config.updated", value.Name)
	writeJSON(w, http.StatusOK, value)
}
