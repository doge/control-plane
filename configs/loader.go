package configs

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/example/control-plane/internal/models"
)

//go:embed */config.json */install.sh */startup.sh
var files embed.FS

func Defaults() ([]models.Config, error) {
	paths, err := fs.Glob(files, "*/config.json")
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	defaults := make([]models.Config, 0, len(paths))
	for _, filename := range paths {
		data, err := files.ReadFile(filename)
		if err != nil {
			return nil, err
		}
		var config models.Config
		if err := json.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("read config %s: %w", filename, err)
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		var definition struct {
			StartupFile string `json:"startupFile"`
			Install     struct {
				ScriptFile string `json:"scriptFile"`
			} `json:"install"`
		}
		if err := json.Unmarshal(raw["spec"], &definition); err != nil {
			return nil, err
		}
		if definition.StartupFile != "" {
			if path.Base(definition.StartupFile) != definition.StartupFile {
				return nil, fmt.Errorf("config %s has invalid startup script path", config.Slug)
			}
			script, err := files.ReadFile(path.Join(path.Dir(filename), definition.StartupFile))
			if err != nil {
				return nil, fmt.Errorf("read startup script for config %s: %w", config.Slug, err)
			}
			config.Spec.Startup = strings.TrimSpace(string(script))
		}
		if definition.Install.ScriptFile != "" {
			if path.Base(definition.Install.ScriptFile) != definition.Install.ScriptFile {
				return nil, fmt.Errorf("config %s has invalid install script path", config.Slug)
			}
			script, err := files.ReadFile(path.Join(path.Dir(filename), definition.Install.ScriptFile))
			if err != nil {
				return nil, fmt.Errorf("read install script for config %s: %w", config.Slug, err)
			}
			if config.Spec.Install == nil {
				return nil, fmt.Errorf("config %s declares an install script without an install container", config.Slug)
			}
			config.Spec.Install.Script = strings.TrimSpace(string(script))
		}
		if config.Name == "" || config.Slug == "" {
			return nil, fmt.Errorf("config %s requires name and slug", filename)
		}
		defaults = append(defaults, config)
	}
	return defaults, nil
}
