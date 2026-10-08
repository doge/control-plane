package controller

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/example/control-plane/internal/models"
	"github.com/example/control-plane/internal/resources"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const minimumVolumeBytes = resources.MinimumVolumeBytes

func (c *Controller) serverVolumes(w http.ResponseWriter, r *http.Request, server models.Server, tail []string) {
	if len(tail) == 0 && r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]any{"diskSizeBytes": server.DiskSizeBytes, "volumes": server.Volumes})
		return
	}
	if len(tail) == 0 && r.Method == http.MethodPost {
		c.createServerVolume(w, r, server)
		return
	}
	if len(tail) == 1 && r.Method == http.MethodPatch {
		c.resizeServerVolume(w, r, server, tail[0])
		return
	}
	if len(tail) == 1 && r.Method == http.MethodDelete {
		c.deleteServerVolume(w, r, server, tail[0])
		return
	}
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
}

func (c *Controller) createServerVolume(w http.ResponseWriter, r *http.Request, server models.Server) {
	var in struct {
		Name      string `json:"name"`
		MountPath string `json:"mountPath"`
		SizeBytes int64  `json:"sizeBytes"`
	}
	if bodyJSON(r, &in) != nil || strings.TrimSpace(in.Name) == "" || in.SizeBytes < minimumVolumeBytes {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "volume name and size (at least 128 MB) are required"})
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.MountPath = strings.TrimSpace(in.MountPath)
	var config models.Config
	if err := c.service.Configs.FindOne(r.Context(), bson.M{"_id": server.ConfigID}).Decode(&config); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "server Config not found"})
		return
	}
	dataPath := config.Spec.DataDirectory
	if dataPath == "" {
		dataPath = "/home/container"
	}
	if !validVolumeMountPath(in.MountPath, dataPath) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "mount path must be a normalized path under /mnt or inside the server data directory"})
		return
	}
	for _, existing := range server.Volumes {
		if strings.EqualFold(existing.Name, in.Name) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "a volume with this name already exists on the server"})
			return
		}
		if volumePathsOverlap(existing.MountPath, in.MountPath) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "volume mount paths cannot overlap"})
			return
		}
	}
	nodeConn, ok := c.nodeForServer(w, r, server)
	if !ok {
		return
	}
	volume := models.ServerVolume{ID: bson.NewObjectID().Hex(), Name: in.Name, MountPath: in.MountPath, SizeBytes: in.SizeBytes, CreatedAt: time.Now()}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	if _, err := nodeConn.request(ctx, map[string]any{
		"type": "server_volume_create", "serverId": server.ID.Hex(),
		"payload": map[string]any{"volumeId": volume.ID, "sizeBytes": volume.SizeBytes},
	}); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	server.Volumes = append(server.Volumes, volume)
	if _, err := c.service.Servers.UpdateOne(r.Context(), bson.M{"_id": server.ID}, bson.M{"$set": bson.M{"volumes": server.Volumes}}); err != nil {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Minute)
		_, _ = nodeConn.request(cleanupCtx, map[string]any{
			"type": "server_volume_delete", "serverId": server.ID.Hex(),
			"payload": map[string]any{"volumeId": volume.ID},
		})
		cleanupCancel()
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "volume was created on the node but could not be saved to the server"})
		return
	}
	if server.ContainerID != "" {
		if err := c.redeployWithVolumes(r.Context(), nodeConn, &server); err != nil {
			server.ContainerID, server.Status = "", "error"
			_, _ = c.service.Servers.UpdateOne(context.Background(), bson.M{"_id": server.ID}, bson.M{"$set": bson.M{"containerId": "", "status": "error", "updatedAt": time.Now()}})
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "volume was created but the server could not be redeployed: " + err.Error()})
			return
		}
		if err := c.saveServerVolumeState(r.Context(), server); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server was redeployed but its state could not be saved"})
			return
		}
	}
	c.recordAudit(r, "server.volume.created", server.Name+" / "+volume.Name)
	writeJSON(w, http.StatusCreated, map[string]any{"volume": volume})
}

func (c *Controller) resizeServerVolume(w http.ResponseWriter, r *http.Request, server models.Server, volumeID string) {
	var in struct {
		SizeBytes int64 `json:"sizeBytes"`
	}
	if bodyJSON(r, &in) != nil || in.SizeBytes < minimumVolumeBytes {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "a volume size of at least 128 MB is required"})
		return
	}
	isPrimary := volumeID == "data"
	if isPrimary {
		if server.DiskSizeBytes > 0 && in.SizeBytes <= server.DiskSizeBytes {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "volumes can be extended but cannot be reduced"})
			return
		}
		server.DiskSizeBytes = in.SizeBytes
	} else {
		found := false
		for i := range server.Volumes {
			if server.Volumes[i].ID != volumeID {
				continue
			}
			if in.SizeBytes <= server.Volumes[i].SizeBytes {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "volumes can be extended but cannot be reduced"})
				return
			}
			server.Volumes[i].SizeBytes = in.SizeBytes
			found = true
			break
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "volume not found"})
			return
		}
	}
	nodeConn, ok := c.nodeForServer(w, r, server)
	if !ok {
		return
	}
	if server.ContainerID != "" {
		if err := c.redeployWithVolumes(r.Context(), nodeConn, &server); err != nil {
			server.ContainerID, server.Status = "", "error"
			_, _ = c.service.Servers.UpdateOne(context.Background(), bson.M{"_id": server.ID}, bson.M{"$set": bson.M{"containerId": "", "status": "error", "updatedAt": time.Now()}})
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "volume size could not be applied during server redeploy: " + err.Error()})
			return
		}
	} else {
		nodeVolumeID := volumeID
		if isPrimary {
			nodeVolumeID = server.ID.Hex()
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
		defer cancel()
		if _, err := nodeConn.request(ctx, map[string]any{
			"type": "server_volume_resize", "serverId": server.ID.Hex(),
			"payload": map[string]any{"volumeId": nodeVolumeID, "sizeBytes": in.SizeBytes},
		}); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
	}
	if err := c.saveServerVolumeState(r.Context(), server); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	c.recordAudit(r, "server.volume.extended", server.Name+" / "+volumeID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (c *Controller) deleteServerVolume(w http.ResponseWriter, r *http.Request, server models.Server, volumeID string) {
	index := -1
	for i := range server.Volumes {
		if server.Volumes[i].ID == volumeID {
			index = i
			break
		}
	}
	if index < 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "attached volume not found"})
		return
	}
	volume := server.Volumes[index]
	nodeConn, ok := c.nodeForServer(w, r, server)
	if !ok {
		return
	}
	candidate := server
	candidate.Volumes = append(append([]models.ServerVolume(nil), server.Volumes[:index]...), server.Volumes[index+1:]...)
	if server.ContainerID != "" {
		if err := c.redeployWithVolumes(r.Context(), nodeConn, &candidate); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "volume could not be detached because the server could not be redeployed: " + err.Error()})
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	if _, err := nodeConn.request(ctx, map[string]any{
		"type": "server_volume_delete", "serverId": server.ID.Hex(),
		"payload": map[string]any{"volumeId": volume.ID},
	}); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "volume was detached but could not be removed: " + err.Error()})
		return
	}
	if _, err := c.service.Servers.UpdateOne(r.Context(), bson.M{"_id": server.ID}, bson.M{"$set": bson.M{
		"volumes": candidate.Volumes, "containerId": candidate.ContainerID, "status": candidate.Status, "updatedAt": time.Now(),
	}}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "volume data was removed but server state could not be saved"})
		return
	}
	c.recordAudit(r, "server.volume.deleted", server.Name+" / "+volume.Name)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (c *Controller) redeployWithVolumes(ctx context.Context, nodeConn *nodeSession, server *models.Server) error {
	var config models.Config
	if err := c.service.Configs.FindOne(ctx, bson.M{"_id": server.ConfigID}).Decode(&config); err != nil {
		return err
	}
	payload := BuildServerCreatePayload(*server, config)
	payload["action"] = "deploy"
	deployCtx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	result, err := nodeConn.request(deployCtx, map[string]any{
		"type": "server_action", "serverId": server.ID.Hex(), "containerId": server.ContainerID,
		"payload": payload,
	})
	if err != nil {
		return err
	}
	response, _ := result["payload"].(map[string]any)
	server.ContainerID, _ = response["containerId"].(string)
	if server.ContainerID == "" {
		return fmt.Errorf("node did not return a container ID")
	}
	server.Status, server.UpdatedAt = "running", time.Now()
	return nil
}

func (c *Controller) saveServerVolumeState(ctx context.Context, server models.Server) error {
	_, err := c.service.Servers.UpdateOne(ctx, bson.M{"_id": server.ID}, bson.M{"$set": bson.M{
		"diskSizeBytes": server.DiskSizeBytes, "volumes": server.Volumes,
		"containerId": server.ContainerID, "status": server.Status, "updatedAt": server.UpdatedAt,
	}})
	return err
}

func validVolumeMountPath(value, dataDirectory string) bool {
	if value == "" || !strings.HasPrefix(value, "/") || value == "/" || path.Clean(value) != value {
		return false
	}
	for _, protected := range []string{"/proc", "/sys", "/dev", "/etc", "/usr", "/bin", "/sbin", "/lib", "/lib64", "/var", "/root"} {
		if value == protected || strings.HasPrefix(value, protected+"/") {
			return false
		}
	}
	if value == dataDirectory {
		return false
	}
	return strings.HasPrefix(value, "/mnt/") || strings.HasPrefix(value, strings.TrimSuffix(dataDirectory, "/")+"/")
}

func volumePathsOverlap(left, right string) bool {
	return left == right || strings.HasPrefix(left, strings.TrimSuffix(right, "/")+"/") || strings.HasPrefix(right, strings.TrimSuffix(left, "/")+"/")
}
