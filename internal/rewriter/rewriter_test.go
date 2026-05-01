package rewriter

import (
	"strings"
	"testing"
)

func TestRewritePathRewritesOnlyNewStoreEntries(t *testing.T) {
	baseline := "/nix/store/system-coreutils/bin:/usr/bin"
	pathValue := "/nix/store/project-gcc/bin:/nix/store/system-coreutils/bin:/usr/bin:/bin"

	got := RewritePath(pathValue, baseline, "/run/user/1000/sandix-store")
	want := "/run/user/1000/sandix-store/project-gcc/bin:/nix/store/system-coreutils/bin:/usr/bin:/bin"

	if got != want {
		t.Fatalf("RewritePath() = %q, want %q", got, want)
	}
}

func TestRewritePathPreservesNonStoreEntries(t *testing.T) {
	got := RewritePath("/home/me/bin:/usr/bin", "/usr/bin", "/sandix")
	if got != "/home/me/bin:/usr/bin" {
		t.Fatalf("RewritePath() = %q", got)
	}
}

func TestRewritePathPreservesEmptyEntries(t *testing.T) {
	got := RewritePath(":/nix/store/project/bin:", "", "/sandix")
	want := ":/sandix/project/bin:"
	if got != want {
		t.Fatalf("RewritePath() = %q, want %q", got, want)
	}
}

func TestAppendPathRewritePreservesInput(t *testing.T) {
	input := []byte("export FOO='bar';")

	got := string(AppendPathRewrite(input, "/usr/bin", "/sandix", "/bin/sandix"))

	if !strings.HasPrefix(got, "export FOO='bar';\n") {
		t.Fatalf("AppendPathRewrite() should preserve original script, got %q", got)
	}
	if !strings.Contains(got, "'/bin/sandix' rewrite --mount-point '/sandix' --baseline '/usr/bin' \"$PATH\"") {
		t.Fatalf("AppendPathRewrite() should append sandix rewrite command, got %q", got)
	}
	if !strings.Contains(got, "export PATH\n") {
		t.Fatalf("AppendPathRewrite() should export rewritten PATH, got %q", got)
	}
}

func TestAppendPathRewriteQuotesShellValues(t *testing.T) {
	got := string(AppendPathRewrite(nil, "/weird ' path", "/sandix ' mount", "/bin/sandix"))

	if !strings.Contains(got, "'/sandix '\"'\"' mount'") {
		t.Fatalf("mount point was not shell quoted correctly: %q", got)
	}
	if !strings.Contains(got, "'/weird '\"'\"' path'") {
		t.Fatalf("baseline was not shell quoted correctly: %q", got)
	}
}
