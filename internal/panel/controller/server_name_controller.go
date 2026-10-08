package controller

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/example/control-plane/internal/models"
	"github.com/example/control-plane/internal/panel/service"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// renameServer updates the display name without recreating the running container.
func (c *Controller) renameServer(w http.ResponseWriter, r *http.Request, server models.Server) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		Name string `json:"name"`
	}
	if bodyJSON(r, &input) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "server name is required"})
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "server name cannot be empty"})
		return
	}
	filter := duplicateServerNameFilter(input.Name)
	filter["_id"] = bson.M{"$ne": server.ID}
	var duplicate models.Server
	if err := c.service.Servers.FindOne(r.Context(), filter).Decode(&duplicate); err == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "A server with this name already exists."})
		return
	} else if !errors.Is(err, service.ErrNotFound) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not check server name availability."})
		return
	}
	updatedAt := time.Now()
	_, err := c.service.Servers.UpdateOne(r.Context(), bson.M{"_id": server.ID}, bson.M{"$set": bson.M{
		"name": input.Name, "nameKey": normalizedServerName(input.Name), "updatedAt": updatedAt,
	}})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "A server with this name already exists."})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	c.recordAudit(r, "server.renamed", server.Name+" → "+input.Name)
	writeJSON(w, http.StatusOK, map[string]string{"id": server.ID.Hex(), "name": input.Name})
}
