package rewriter

import (
	"strings"
	"testing"
)

func TestRewritePATH(t *testing.T) {
	input := `export PATH='/nix/store/abc-gcc-12.3/bin:/nix/store/xyz-coreutils-9.4/bin:/usr/bin'`
	result := string(Rewrite([]byte(input), "/nix/.sandboxed-store"))

	if !strings.Contains(result, "/nix/.sandboxed-store/abc-gcc-12.3/bin") {
		t.Error("should rewrite nix store paths in PATH")
	}
	if !strings.Contains(result, "/nix/.sandboxed-store/xyz-coreutils-9.4/bin") {
		t.Error("should rewrite all nix store paths in PATH")
	}
	if !strings.Contains(result, ":/usr/bin") {
		t.Error("should preserve non-store PATH entries")
	}
}

func TestRewritePreservesOtherVars(t *testing.T) {
	input := `NIX_CFLAGS_COMPILE=' -isystem /nix/store/abc-openssl-3.6.1-dev/include'
export NIX_CFLAGS_COMPILE
PKG_CONFIG_PATH='/nix/store/abc-openssl-3.6.1-dev/lib/pkgconfig'
export PKG_CONFIG_PATH
PATH='/nix/store/abc-gcc-12.3/bin:/usr/bin'
export PATH`

	result := string(Rewrite([]byte(input), "/nix/.sandboxed-store"))

	if strings.Contains(result, "/nix/.sandboxed-store/abc-openssl") {
		t.Error("should NOT rewrite NIX_CFLAGS_COMPILE or PKG_CONFIG_PATH")
	}
	if !strings.Contains(result, "/nix/.sandboxed-store/abc-gcc-12.3/bin") {
		t.Error("should rewrite PATH")
	}
}

func TestRewriteDeclareSyntax(t *testing.T) {
	input := `declare -x PATH="/nix/store/abc-gcc-12.3/bin:/usr/bin"`
	result := string(Rewrite([]byte(input), "/nix/.sandboxed-store"))

	if !strings.Contains(result, "/nix/.sandboxed-store/abc-gcc-12.3/bin") {
		t.Error("should handle declare -x PATH=")
	}
}

func TestRewriteBarePATH(t *testing.T) {
	input := `PATH='/nix/store/abc-gcc-12.3/bin'`
	result := string(Rewrite([]byte(input), "/nix/.sandboxed-store"))

	if !strings.Contains(result, "/nix/.sandboxed-store/abc-gcc-12.3/bin") {
		t.Error("should handle bare PATH=")
	}
}

func TestRewriteEmptyInput(t *testing.T) {
	result := Rewrite([]byte(""), "/nix/.sandboxed-store")
	if string(result) != "" {
		t.Errorf("empty input should produce empty output, got %q", string(result))
	}
}

func TestRewriteNoPATH(t *testing.T) {
	input := `export FOO=bar
export BAZ=qux`
	result := string(Rewrite([]byte(input), "/nix/.sandboxed-store"))
	if result != input {
		t.Errorf("input without PATH should pass through unchanged, got %q", result)
	}
}

func TestRewriteDoesNotMatchBinFiles(t *testing.T) {
	input := `BASH='/nix/store/abc-bash-5.3/bin/bash'`
	result := string(Rewrite([]byte(input), "/nix/.sandboxed-store"))
	if strings.Contains(result, "sandboxed-store") {
		t.Error("should NOT rewrite /nix/store/.../bin/bash (file reference)")
	}
}
