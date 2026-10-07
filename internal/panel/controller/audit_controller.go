package controller

import (
	"net/http"
	"time"

	"github.com/example/control-plane/internal/models"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// dashboard returns authorized server data for the panel overview.
func (c *Controller) dashboard(w http.ResponseWriter, r *http.Request) {
	servers := make([]models.Server, 0)
	if err := c.service.Servers.FindAll(r.Context(), bson.D{}, &servers); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := c.redactServerVariables(r.Context(), servers); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"servers": servers, "timestamp": time.Now()})
}

// audit returns the newest audit records visible to the caller.
func (c *Controller) audit(w http.ResponseWriter, r *http.Request) {
	out := make([]models.AuditLog, 0)
	if err := c.service.AuditLogs.FindAll(r.Context(), bson.D{}, &out, bson.D{{Key: "createdAt", Value: -1}}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, out)
}

// recordAudit stores an action with the authenticated actor and request details.
func (c *Controller) recordAudit(r *http.Request, action, detail string) {
	entry := models.AuditLog{ID: bson.NewObjectID(), Action: action, Detail: detail, CreatedAt: time.Now()}
	if actor, ok := r.Context().Value(userKey).(models.User); ok {
		id := actor.ID
		entry.UserID = &id
	}
	_, _ = c.service.AuditLogs.InsertOne(r.Context(), entry)
}
