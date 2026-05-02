package wrapper

import (
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	result := string(Generate("abc123-gcc-12.3", "gcc", "/usr/bin/sandbox-exec"))

	if !strings.HasPrefix(result, "#!/bin/sh\n") {
		t.Error("wrapper should start with shebang")
	}
	if !strings.Contains(result, "/usr/bin/sandbox-exec") {
		t.Error("wrapper should contain sandbox-exec path")
	}
	if !strings.Contains(result, "/nix/store/abc123-gcc-12.3/bin/gcc") {
		t.Error("wrapper should contain full binary path")
	}
	if strings.Contains(result, "landrun") {
		t.Error("wrapper should delegate sandbox policy to sandbox-exec")
	}
	if !strings.Contains(result, "SANDIX_PATH") {
		t.Error("wrapper should use the devshell PATH before entering sandbox-exec")
	}
	if !strings.Contains(result, `"$@"`) {
		t.Error("wrapper should pass through arguments")
	}
}

func TestGenerateDeterministic(t *testing.T) {
	a := Generate("abc-foo-1.0", "foo", "/bin/sandbox-exec")
	b := Generate("abc-foo-1.0", "foo", "/bin/sandbox-exec")
	if string(a) != string(b) {
		t.Error("Generate should be deterministic")
	}
}
