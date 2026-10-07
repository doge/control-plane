package panel_test

import (
	"strings"
	"testing"

	configfiles "github.com/example/control-plane/configs"
	"github.com/example/control-plane/internal/models"
	"github.com/example/control-plane/internal/panel/controller"
)

func TestConfigPayloadSeparatesInstallFromRuntimeLaunch(t *testing.T) {
	defaults, err := configfiles.Defaults()
	if err != nil {
		t.Fatal(err)
	}
	bySlug := make(map[string]models.Config, len(defaults))
	for _, value := range defaults {
		bySlug[value.Slug] = value
	}
	paper := controller.BuildServerCreatePayload(models.Server{Name: "paper", MemoryMB: 2048}, bySlug["paper-minecraft"])
	if paper["launchCommand"] != bySlug["paper-minecraft"].Spec.Startup {
		t.Fatal("Paper launch command was not sent to the runtime container")
	}
	if len(paper["entrypoint"].([]any)) == 0 || len(paper["command"].([]any)) == 0 {
		t.Fatal("Paper must use its direct Java launch command")
	}
	install := paper["install"].(map[string]any)
	if install["script"] == "" || strings.Contains(install["script"].(string), "{{") {
		t.Fatal("Config install script should be sent separately and without startup interpolation")
	}
	cs2 := controller.BuildServerCreatePayload(models.Server{Name: "cs2"}, bySlug["cs2"])
	if cs2["launchCommand"] != "" || len(cs2["command"].([]any)) != 0 || len(cs2["entrypoint"].([]any)) != 0 {
		t.Fatal("CS2 must preserve its upstream image entrypoint")
	}
}
