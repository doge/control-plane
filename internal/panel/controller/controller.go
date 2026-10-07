package controller

import (
	"net/http"
	"time"

	"github.com/example/control-plane/internal/panel/service"
)

// Controller adapts HTTP requests to application services and formats responses.
type Controller struct {
	app     *App
	service *service.Service
}

// health returns a lightweight liveness response for the REST API.
func (c *Controller) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "time": time.Now()})
}
