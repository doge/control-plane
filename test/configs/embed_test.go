package configs_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	configfiles "github.com/example/control-plane/configs"
	"github.com/example/control-plane/internal/config"
)

func TestBuiltInConfigsAreValidAndModular(t *testing.T) {
	defaults, err := configfiles.Defaults()
	if err != nil {
		t.Fatal(err)
	}
	if len(defaults) != 7 {
		t.Fatalf("configfiles.Defaults() returned %d Configs, want 7", len(defaults))
	}
	bySlug := make(map[string]int, len(defaults))
	for _, value := range defaults {
		bySlug[value.Slug]++
		if err := config.Validate(value); err != nil {
			t.Errorf("Config %s is invalid: %v", value.Slug, err)
		}
		if len(value.Spec.DockerImages) == 0 {
			t.Errorf("Config %s has no Docker image", value.Slug)
		}
		if value.Spec.Resources.CPULimit <= 0 || value.Spec.Resources.MemoryMB < 256 {
			t.Errorf("Config %s has no useful default resource limits", value.Slug)
		}
		if len(value.Spec.Ports) == 0 {
			t.Errorf("Config %s has no ports", value.Slug)
		}
		if value.Spec.Install != nil && value.Spec.Install.Script == "" {
			t.Errorf("Config %s has no install script", value.Slug)
		}
	}
	for _, slug := range []string{"paper-minecraft", "minecraft-java", "fabric-minecraft", "cs2", "dayz", "rust", "steamcmd-template"} {
		if bySlug[slug] != 1 {
			t.Errorf("expected one built-in Config with slug %q", slug)
		}
	}
}

func TestVanillaMinecraftConfigDownloadsJarUsedByStartup(t *testing.T) {
	defaults, err := configfiles.Defaults()
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range defaults {
		if value.Slug != "minecraft-java" {
			continue
		}
		if value.Spec.Install == nil || !strings.Contains(value.Spec.Install.Script, "piston-meta.mojang.com") {
			t.Fatal("Vanilla Config does not install from Mojang's version manifest")
		}
		if !strings.Contains(value.Spec.Install.Script, "/mnt/server/${SERVER_JARFILE}") || value.Spec.Startup != "java -Xms128M -XX:MaxRAMPercentage=75 -jar /data/{{SERVER_JARFILE}} --nogui" {
			t.Fatal("Vanilla installer and startup command do not use the same jar variable")
		}
		return
	}
	t.Fatal("Vanilla Minecraft Config missing")
}

func TestVanillaMinecraftStartupExecutesConfiguredJar(t *testing.T) {
	defaults, err := configfiles.Defaults()
	if err != nil {
		t.Fatal(err)
	}
	var startup string
	for _, value := range defaults {
		if value.Slug == "minecraft-java" {
			startup = strings.ReplaceAll(value.Spec.Startup, "{{SERVER_JARFILE}}", "server.jar")
			break
		}
	}
	if startup == "" {
		t.Fatal("Vanilla Minecraft Config missing")
	}

	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeJava := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$JAVA_ARGS\"\n"
	if err := os.WriteFile(filepath.Join(bin, "java"), []byte(fakeJava), 0o755); err != nil {
		t.Fatal(err)
	}
	argsFile := filepath.Join(dir, "java-args")
	cmd := exec.Command("bash", "-c", startup)
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "JAVA_ARGS="+argsFile)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Vanilla startup failed: %v\n%s", err, output)
	}
	got, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	want := "-Xms128M\n-XX:MaxRAMPercentage=75\n-jar\n/data/server.jar\n--nogui\n"
	if string(got) != want {
		t.Fatalf("Java arguments = %q, want %q", got, want)
	}
}

func TestPaperHasSeparateInstallAndSimpleRuntimeCommand(t *testing.T) {
	defaults, err := configfiles.Defaults()
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range defaults {
		if value.Slug != "paper-minecraft" {
			continue
		}
		if value.Spec.Install == nil || !strings.Contains(value.Spec.Install.Script, "fill.papermc.io/v3") {
			t.Fatal("Paper Config has no separate installer using the supported downloads API")
		}
		if !strings.Contains(value.Spec.Install.Script, "/mnt/server/${SERVER_JARFILE}") || !strings.Contains(value.Spec.Startup, "/data/{{SERVER_JARFILE}}") {
			t.Fatal("Paper installer and startup command do not use the same jar variable")
		}
		if !strings.HasPrefix(value.Spec.Startup, "java ") || strings.Contains(value.Spec.Startup, "mc-image-helper") {
			t.Fatalf("Paper runtime command should be direct Java launch: %q", value.Spec.Startup)
		}
		if value.Spec.Resources.MemoryMB != 2048 {
			t.Fatalf("Paper default memory = %d MB, want 2048 MB", value.Spec.Resources.MemoryMB)
		}
		return
	}
	t.Fatal("Paper Config missing")
}

func TestImageNativeAndX86OnlyConfigs(t *testing.T) {
	defaults, err := configfiles.Defaults()
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range defaults {
		switch value.Slug {
		case "cs2":
			if value.Spec.Startup != "" {
				t.Error("CS2 must inherit the image's entrypoint and command")
			}
			if !strings.Contains(value.Spec.DockerImages[0], "joedwards32/cs2") {
				t.Errorf("unexpected CS2 image: %s", value.Spec.DockerImages[0])
			}
		case "dayz", "rust":
			if len(value.Spec.SupportedArchitectures) != 1 || value.Spec.SupportedArchitectures[0] != "amd64" {
				t.Errorf("%s must require amd64 for its SteamCMD binaries", value.Slug)
			}
			if value.Spec.Install == nil || value.Spec.Startup == "" {
				t.Errorf("%s needs distinct install and startup scripts", value.Slug)
			}
		}
	}
}
