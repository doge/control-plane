package controller

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/example/control-plane/internal/models"
)

const uploadChunkBytes = 3 << 20

// uploadServerFile streams a multipart upload to the node in bounded chunks.
func (c *Controller) uploadServerFile(w http.ResponseWriter, r *http.Request, server models.Server, root, destination string, source io.Reader, size int64) {
	if server.ContainerID == "" {
		writeJSON(w, 409, map[string]string{"error": "server has no container id; deploy it first"})
		return
	}
	node, ok := c.nodeForServer(w, r, server)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	uploadID := randomToken(16)
	request := func(kind string, payload map[string]any) error {
		payload["root"] = root
		result, err := node.request(ctx, map[string]any{
			"type": kind, "serverId": server.ID.Hex(), "containerId": server.ContainerID, "payload": payload,
		})
		if err != nil {
			return err
		}
		if resultPayload, ok := result["payload"].(map[string]any); ok {
			if message, _ := resultPayload["error"].(string); message != "" {
				return fmt.Errorf("%s", message)
			}
		}
		return nil
	}
	abort := func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		cleanupPayload := map[string]any{"uploadId": uploadID}
		cleanupPayload["root"] = root
		_, _ = node.request(cleanupCtx, map[string]any{
			"type": "file_upload_abort", "serverId": server.ID.Hex(), "containerId": server.ContainerID, "payload": cleanupPayload,
		})
	}
	if err := request("file_upload_begin", map[string]any{
		"uploadId": uploadID, "path": destination, "size": size,
	}); err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	complete := false
	defer func() {
		if !complete {
			abort()
		}
	}()
	buffer := make([]byte, uploadChunkBytes)
	var offset int64
	for {
		read, readErr := source.Read(buffer)
		if read > 0 {
			chunk := base64.StdEncoding.EncodeToString(buffer[:read])
			if err := request("file_upload_chunk", map[string]any{
				"uploadId": uploadID, "offset": strconv.FormatInt(offset, 10), "data": chunk,
			}); err != nil {
				writeJSON(w, 502, map[string]string{"error": err.Error()})
				return
			}
			offset += int64(read)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			writeJSON(w, 500, map[string]string{"error": "could not read uploaded file"})
			return
		}
	}
	if size >= 0 && offset != size {
		writeJSON(w, 500, map[string]string{"error": "uploaded file size did not match the received data"})
		return
	}
	if err := request("file_upload_finish", map[string]any{
		"uploadId": uploadID, "path": destination, "size": strconv.FormatInt(offset, 10),
	}); err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	complete = true
	writeJSON(w, 200, map[string]any{"ok": true, "bytes": offset})
}
