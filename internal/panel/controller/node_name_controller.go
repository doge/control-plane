package controller

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/example/control-plane/internal/models"
	"github.com/example/control-plane/internal/panel/service"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

var nodeNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._ -]{0,63}$`)

func normalizedNodeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func validNodeName(name string) bool {
	return nodeNamePattern.MatchString(name)
}

func nodeNameFilter(name string, exceptID bson.ObjectID) bson.M {
	filter := bson.M{
		"$or": bson.A{
			bson.M{"nameKey": normalizedNodeName(name)},
			bson.M{"name": bson.M{
				"$regex":   "^" + regexp.QuoteMeta(strings.TrimSpace(name)) + "$",
				"$options": "i",
			}},
		},
	}
	if !exceptID.IsZero() {
		filter["_id"] = bson.M{"$ne": exceptID}
	}
	return filter
}

func (c *Controller) renameNode(w http.ResponseWriter, r *http.Request, id bson.ObjectID) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if !requirePermission(w, r, "nodes.manage") {
		return
	}
	var input struct {
		Name string `json:"name"`
	}
	if bodyJSON(r, &input) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node name is required"})
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if !validNodeName(input.Name) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node names must start with a letter or number and use only letters, numbers, spaces, dots, underscores, or hyphens (up to 64 characters)"})
		return
	}
	var current models.Node
	if err := c.service.Nodes.FindOne(r.Context(), bson.M{"_id": id}).Decode(&current); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "node not found"})
		return
	}
	c.app.nodeNamesMu.Lock()
	defer c.app.nodeNamesMu.Unlock()
	var duplicate models.Node
	if err := c.service.Nodes.FindOne(r.Context(), nodeNameFilter(input.Name, id)).Decode(&duplicate); err == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "A node with this name already exists."})
		return
	} else if !errors.Is(err, service.ErrNotFound) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not check node name availability."})
		return
	}
	updatedAt := time.Now()
	_, err := c.service.Nodes.UpdateOne(r.Context(), bson.M{"_id": id}, bson.M{"$set": bson.M{
		"name": input.Name, "nameKey": normalizedNodeName(input.Name), "updatedAt": updatedAt,
	}})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "A node with this name already exists."})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	var node models.Node
	if err := c.service.Nodes.FindOne(r.Context(), bson.M{"_id": id}).Decode(&node); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "node not found"})
		return
	}
	c.recordAudit(r, "node.renamed", current.Name+" → "+input.Name)
	writeJSON(w, http.StatusOK, node)
}
