package node_test

import (
	"testing"

	"github.com/example/control-plane/internal/node"
)

func TestSafePathAtUsesConfiguredServerRoot(t *testing.T) {
	root := "/home/container"
	if path, err := node.SafePathAt("", root); err != nil || path != root {
		t.Fatalf("empty path = %q, %v", path, err)
	}
	if path, err := node.SafePathAt("plugins/example.jar", root); err != nil || path != root+"/plugins/example.jar" {
		t.Fatalf("relative path = %q, %v", path, err)
	}
	for _, path := range []string{"/etc/passwd", "../../etc/passwd", root + "/../outside"} {
		if _, err := node.SafePathAt(path, root); err == nil {
			t.Errorf("path %q escaped the Config data directory", path)
		}
	}
}
