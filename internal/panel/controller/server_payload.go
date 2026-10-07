package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/example/control-plane/internal/models"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func publicServerVariables(value models.Config, variables map[string]string) map[string]string {
	secrets := make(map[string]bool)
	for _, definition := range value.Spec.Variables {
		if definition.Type == "secret" || definition.Secret {
			secrets[definition.Name] = true
			secrets[strings.ToUpper(definition.Name)] = true
		}
	}
	public := make(map[string]string, len(variables))
	for name, item := range variables {
		if !secrets[name] {
			public[name] = item
		}
	}
	return public
}

func (c *Controller) redactServerVariables(ctx context.Context, servers []models.Server) error {
	ids := make([]bson.ObjectID, 0, len(servers))
	seen := make(map[bson.ObjectID]bool, len(servers))
	for _, server := range servers {
		if !seen[server.ConfigID] {
			seen[server.ConfigID] = true
			ids = append(ids, server.ConfigID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	values := make([]models.Config, 0, len(ids))
	if err := c.service.Configs.FindAll(ctx, bson.M{"_id": bson.M{"$in": ids}}, &values); err != nil {
		return err
	}
	byID := make(map[bson.ObjectID]models.Config, len(values))
	for _, value := range values {
		byID[value.ID] = value
	}
	for i := range servers {
		value, ok := byID[servers[i].ConfigID]
		if !ok {
			return fmt.Errorf("Config not found for server %q", servers[i].Name)
		}
		servers[i].Variables = publicServerVariables(value, servers[i].Variables)
	}
	return nil
}

// BuildServerCreatePayload creates the node-agent payload from server and Config data.
func BuildServerCreatePayload(server models.Server, value models.Config) map[string]any {
	spec := value.Spec
	dataDirectory := spec.DataDirectory
	if dataDirectory == "" {
		dataDirectory = "/home/container"
	}
	vars := make(map[string]string, len(server.Variables)+len(server.Allocations)+6)
	for key, item := range server.Variables {
		vars[key] = item
		vars[strings.ToUpper(key)] = item
	}
	vars["SERVER_NAME"] = server.Name
	vars["SERVER_MEMORY"] = fmt.Sprint(server.MemoryMB)
	vars["SERVER_IP"] = server.Address
	vars["SERVER_UUID"] = server.ID.Hex()
	if len(server.Allocations) > 0 {
		vars["SERVER_PORT"] = fmt.Sprint(server.Allocations[0].Container)
		for _, allocation := range server.Allocations {
			name := portVariableName(allocation.Name)
			if name != "" {
				vars[name+"_PORT"] = fmt.Sprint(allocation.Container)
			}
		}
	}

	environment := make(map[string]string, len(spec.Environment)+len(vars))
	for name, raw := range spec.Environment {
		environment[name] = resolveEnvironmentValue(raw, vars)
	}
	for name, item := range vars {
		if name == strings.ToUpper(name) {
			environment[name] = item
		}
	}
	if _, exists := environment["HOME"]; !exists && spec.User == "steam" {
		environment["HOME"] = "/home/steam"
	}
	containerEnv := make([]string, 0, len(environment))
	for name, item := range environment {
		containerEnv = append(containerEnv, name+"="+item)
	}

	image := ""
	if len(spec.DockerImages) > 0 {
		image = spec.DockerImages[0]
	}
	launch := server.LaunchCommand
	if launch == "" {
		launch = spec.Startup
	}
	workingDirectory := spec.WorkingDirectory
	var entrypoint, command []any
	if launch != "" {
		if workingDirectory == "" {
			workingDirectory = dataDirectory
		}
		entrypoint = []any{"/bin/bash", "-lc"}
		command = []any{resolveStartup(launch, vars)}
	}
	var install any
	if spec.Install != nil {
		install = map[string]any{
			"image": spec.Install.Image, "entrypoint": spec.Install.Entrypoint,
			"script": spec.Install.Script,
		}
	}
	return map[string]any{
		"name": server.Name, "image": image, "environment": containerEnv,
		"ports": allocatedPorts(server.Allocations), "memoryMB": server.MemoryMB,
		"diskSizeBytes": server.DiskSizeBytes, "volumes": server.Volumes,
		"cpuLimit": server.CPULimit, "command": command, "entrypoint": entrypoint,
		"launchCommand": launch, "workingDir": workingDirectory, "dataDirectory": dataDirectory,
		"install": install, "user": spec.User,
	}
}

func allocatedPorts(allocations []models.PortAllocation) []any {
	ports := make([]any, 0, len(allocations))
	for _, allocation := range allocations {
		ports = append(ports, map[string]any{
			"name": allocation.Name, "container": allocation.Container,
			"host": allocation.Host, "hostIP": allocation.IP, "protocol": allocation.Protocol,
		})
	}
	return ports
}

func resolveEnvironmentValue(value string, variables map[string]string) string {
	for name, replacement := range variables {
		value = strings.ReplaceAll(value, "{{"+name+"}}", replacement)
		value = strings.ReplaceAll(value, "{{"+strings.ToUpper(name)+"}}", replacement)
	}
	return value
}

func resolveStartup(value string, variables map[string]string) string {
	for name, replacement := range variables {
		quoted := "'" + strings.ReplaceAll(replacement, "'", "'\\''") + "'"
		value = strings.ReplaceAll(value, "{{"+name+"}}", quoted)
		value = strings.ReplaceAll(value, "{{"+strings.ToUpper(name)+"}}", quoted)
	}
	return value
}

func portVariableName(name string) string {
	var result strings.Builder
	for _, char := range strings.ToUpper(strings.TrimSpace(name)) {
		if char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' {
			result.WriteRune(char)
		} else if result.Len() > 0 && !strings.HasSuffix(result.String(), "_") {
			result.WriteByte('_')
		}
	}
	return strings.Trim(result.String(), "_")
}
