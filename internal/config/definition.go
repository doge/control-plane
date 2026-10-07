package config

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/example/control-plane/internal/models"
)

var variableName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func Validate(value models.Config) error {
	spec := value.Spec
	if strings.TrimSpace(value.Name) == "" || !variableName.MatchString(strings.ReplaceAll(value.Slug, "-", "_")) {
		return fmt.Errorf("config name and a valid slug are required")
	}
	if len(spec.DockerImages) == 0 || strings.TrimSpace(spec.DockerImages[0]) == "" {
		return fmt.Errorf("config needs at least one Docker image")
	}
	if spec.Version != 1 {
		return fmt.Errorf("unsupported Config definition version %d", spec.Version)
	}
	for _, image := range spec.DockerImages {
		if strings.TrimSpace(image) == "" {
			return fmt.Errorf("Config Docker image list cannot contain empty entries")
		}
	}
	if spec.Install != nil && (strings.TrimSpace(spec.Install.Image) == "" || strings.TrimSpace(spec.Install.Script) == "") {
		return fmt.Errorf("install image and script are both required")
	}
	if spec.DataDirectory != "" && (!path.IsAbs(spec.DataDirectory) || path.Clean(spec.DataDirectory) == "/") {
		return fmt.Errorf("dataDirectory must be an absolute non-root path")
	}
	if spec.WorkingDirectory != "" && !path.IsAbs(spec.WorkingDirectory) {
		return fmt.Errorf("workingDirectory must be absolute")
	}
	seen := make(map[string]bool, len(spec.Variables))
	for _, variable := range spec.Variables {
		if !variableName.MatchString(variable.Name) || seen[strings.ToUpper(variable.Name)] {
			return fmt.Errorf("config variable names must be unique environment variable names")
		}
		seen[strings.ToUpper(variable.Name)] = true
		if variable.Type != "" && variable.Type != "text" && variable.Type != "integer" && variable.Type != "secret" {
			return fmt.Errorf("variable %s has an unsupported type", variable.Name)
		}
	}
	for _, port := range spec.Ports {
		protocol := strings.ToLower(port.Protocol)
		if port.Container < 1 || port.Container > 65535 || (protocol != "tcp" && protocol != "udp" && protocol != "sctp") {
			return fmt.Errorf("config port %s must specify a valid port and tcp/udp/sctp protocol", port.Name)
		}
	}
	return nil
}

func SupportsArchitecture(spec models.ConfigSpec, architecture string) bool {
	if len(spec.SupportedArchitectures) == 0 {
		return true
	}
	for _, supported := range spec.SupportedArchitectures {
		if strings.EqualFold(supported, architecture) {
			return true
		}
	}
	return false
}

func DefaultValues(spec models.ConfigSpec, input map[string]string) (map[string]string, error) {
	values := make(map[string]string, len(spec.Variables)+5)
	for _, variable := range spec.Variables {
		value, exists := input[variable.Name]
		if !exists {
			for key, candidate := range input {
				if strings.EqualFold(key, variable.Name) {
					value, exists = candidate, true
					break
				}
			}
		}
		if !exists {
			value = variable.Default
		}
		if variable.Required && strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("required config variable %q is missing", variable.Name)
		}
		values[variable.Name] = value
	}
	return values, nil
}
