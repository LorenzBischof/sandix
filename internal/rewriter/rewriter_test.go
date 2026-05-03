package rewriter

import (
	"strings"
	"testing"
)

func TestRewritePathRewritesStoreEntries(t *testing.T) {
	pathValue := "/nix/store/project-gcc/bin:/nix/store/system-coreutils/bin:/usr/bin:/bin"

	got := RewritePath(pathValue, "/run/user/1000/sandix-store")
	want := "/run/user/1000/sandix-store/project-gcc/bin:/run/user/1000/sandix-store/system-coreutils/bin:/usr/bin:/bin"

	if got != want {
		t.Fatalf("RewritePath() = %q, want %q", got, want)
	}
}

func TestRewritePathPreservesNonStoreEntries(t *testing.T) {
	got := RewritePath("/home/me/bin:/usr/bin", "/sandix")
	if got != "/home/me/bin:/usr/bin" {
		t.Fatalf("RewritePath() = %q", got)
	}
}

func TestRewritePathPreservesEmptyEntries(t *testing.T) {
	got := RewritePath(":/nix/store/project/bin:", "/sandix")
	want := ":/sandix/project/bin:"
	if got != want {
		t.Fatalf("RewritePath() = %q, want %q", got, want)
	}
}

func TestAppendPathRewritePreservesInput(t *testing.T) {
	input := []byte("export FOO='bar';")

	got := string(AppendPathRewrite(input, "/sandix", "/bin/sandix", ""))

	if !strings.HasPrefix(got, "export FOO='bar';\n") {
		t.Fatalf("AppendPathRewrite() should preserve original script, got %q", got)
	}
	if !strings.Contains(got, "'/bin/sandix' direnv-path --mount-point '/sandix'") {
		t.Fatalf("AppendPathRewrite() should append sandix direnv-path command, got %q", got)
	}
	if !strings.Contains(got, "export PATH\n") {
		t.Fatalf("AppendPathRewrite() should export rewritten PATH, got %q", got)
	}
}

func TestAppendPathRewriteQuotesShellValues(t *testing.T) {
	got := string(AppendPathRewrite([]byte("export FOO=bar\n"), "/sandix ' mount", "/bin/sandix", ""))

	if !strings.Contains(got, "'/sandix '\"'\"' mount'") {
		t.Fatalf("mount point was not shell quoted correctly: %q", got)
	}
}

func TestAppendPathRewritePrependsTrustedPathOnlyOutsideSandbox(t *testing.T) {
	got := string(AppendPathRewrite([]byte("export FOO=bar\n"), "/sandix", "/bin/sandix", "/trusted/bin"))

	if !strings.Contains(got, "PATH='/trusted/bin':\"$PATH\"") {
		t.Fatalf("AppendPathRewrite() should prepend trusted path interactively, got %q", got)
	}
}

func TestAppendPathRewriteSkipsEmptyInput(t *testing.T) {
	got := AppendPathRewrite(nil, "/sandix", "/bin/sandix", "/trusted/bin")

	if len(got) != 0 {
		t.Fatalf("AppendPathRewrite() should not emit a rewrite for empty input, got %q", got)
	}
}
