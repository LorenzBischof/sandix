package wrapper

import (
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	result := string(Generate("abc123-gcc-12.3", "gcc", "/usr/bin/landrun"))

	if !strings.HasPrefix(result, "#!/bin/sh\n") {
		t.Error("wrapper should start with shebang")
	}
	if !strings.Contains(result, "/usr/bin/landrun") {
		t.Error("wrapper should contain landrun path")
	}
	if !strings.Contains(result, "/nix/store/abc123-gcc-12.3/bin/gcc") {
		t.Error("wrapper should contain full binary path")
	}
	if !strings.Contains(result, "--env") {
		t.Error("wrapper should pass through environment variables")
	}
	if !strings.Contains(result, "--rox /nix/store") {
		t.Error("wrapper should allow read-only execution from /nix/store")
	}
	if strings.Contains(result, "--ro /etc") {
		t.Error("wrapper should not allow /etc passthrough")
	}
	if !strings.Contains(result, `"$@"`) {
		t.Error("wrapper should pass through arguments")
	}
}

func TestGenerateDeterministic(t *testing.T) {
	a := Generate("abc-foo-1.0", "foo", "/bin/landrun")
	b := Generate("abc-foo-1.0", "foo", "/bin/landrun")
	if string(a) != string(b) {
		t.Error("Generate should be deterministic")
	}
}
