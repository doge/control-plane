package config_test

import (
	"testing"

	"github.com/example/control-plane/internal/config"
	"github.com/example/control-plane/internal/models"
)

func TestDefaultValuesAppliesDefaultsAndAcceptsCaseVariants(t *testing.T) {
	spec := models.ConfigSpec{Variables: []models.ConfigVariable{
		{Name: "VERSION", Default: "latest"},
		{Name: "RCON_PASSWORD", Type: "secret", Required: true},
	}}
	values, err := config.DefaultValues(spec, map[string]string{"rcon_password": "secret-value"})
	if err != nil {
		t.Fatal(err)
	}
	if values["VERSION"] != "latest" || values["RCON_PASSWORD"] != "secret-value" {
		t.Fatalf("unexpected resolved variable values: %#v", values)
	}
}

func TestValidateSupportsImageDefaultsAndSeparateInstallDefinition(t *testing.T) {
	value := models.Config{
		Name: "native", Slug: "native",
		Spec: models.ConfigSpec{
			Version: 1, DockerImages: []string{"example/game:latest"},
			DataDirectory: "/srv/game",
			Install:       &models.ConfigInstall{Image: "debian:stable", Script: "echo install"},
		},
	}
	if err := config.Validate(value); err != nil {
		t.Fatal(err)
	}
}

func TestSupportsArchitecture(t *testing.T) {
	spec := models.ConfigSpec{SupportedArchitectures: []string{"amd64"}}
	if !config.SupportsArchitecture(spec, "AMD64") || config.SupportsArchitecture(spec, "arm64") {
		t.Fatal("architecture requirements were not enforced")
	}
}
