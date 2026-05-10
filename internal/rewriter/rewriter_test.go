package rewriter

import (
	"strings"
	"testing"
)

func TestAppendPathRewritePreservesInput(t *testing.T) {
	input := []byte("export FOO='bar';")

	got := string(AppendPathRewrite(input, "/bin/sandix", ""))

	if !strings.HasPrefix(got, "export FOO='bar';\n") {
		t.Fatalf("AppendPathRewrite() should preserve original script, got %q", got)
	}
	if !strings.Contains(got, "'/bin/sandix' rewrite-direnv") {
		t.Fatalf("AppendPathRewrite() should append sandix rewrite-direnv command, got %q", got)
	}
	if !strings.Contains(got, "eval \"$__sandix_rewrite\"") {
		t.Fatalf("AppendPathRewrite() should eval rewritten environment, got %q", got)
	}
	if !strings.Contains(got, "return \"$__sandix_status\"") {
		t.Fatalf("AppendPathRewrite() should propagate rewrite failures, got %q", got)
	}
}

func TestAppendPathRewriteQuotesShellValues(t *testing.T) {
	got := string(AppendPathRewrite([]byte("export FOO=bar\n"), "/bin/sandix ' path", ""))

	if !strings.Contains(got, "'/bin/sandix '\"'\"' path'") {
		t.Fatalf("sandix path was not shell quoted correctly: %q", got)
	}
}

func TestAppendPathRewritePassesTrustedPackageNames(t *testing.T) {
	got := string(AppendPathRewrite([]byte("export FOO=bar\n"), "/bin/sandix", "coreutils,gnugrep"))

	if !strings.Contains(got, "'/bin/sandix' rewrite-direnv --trusted-package-names 'coreutils,gnugrep'") {
		t.Fatalf("AppendPathRewrite() should pass trusted package names to rewrite-direnv, got %q", got)
	}
}

func TestAppendPathRewriteSkipsEmptyInput(t *testing.T) {
	got := AppendPathRewrite(nil, "/bin/sandix", "/bin/trusted")

	if len(got) != 0 {
		t.Fatalf("AppendPathRewrite() should not emit a rewrite for empty input, got %q", got)
	}
}
