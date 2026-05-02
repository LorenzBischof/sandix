package rewriter

import (
	"path/filepath"
	"strings"
)

const nixStorePrefix = "/nix/store/"

// RewritePath rewrites /nix/store entries that were added after baseline.
// Entries already present in baseline are treated as trusted shell state and
// are preserved.
func RewritePath(pathValue, baseline, mountPoint string) string {
	trusted := pathEntrySet(baseline)
	entries := filepath.SplitList(pathValue)
	for i, entry := range entries {
		if trusted[entry] {
			continue
		}
		entries[i] = rewritePathEntry(entry, mountPoint)
	}
	return strings.Join(entries, string(filepath.ListSeparator))
}

func rewritePathEntry(entry, mountPoint string) string {
	if !strings.HasPrefix(entry, nixStorePrefix) {
		return entry
	}
	return strings.TrimRight(mountPoint, "/") + "/" + strings.TrimPrefix(entry, nixStorePrefix)
}

func pathEntrySet(pathValue string) map[string]bool {
	entries := filepath.SplitList(pathValue)
	set := make(map[string]bool, len(entries))
	for _, entry := range entries {
		set[entry] = true
	}
	return set
}

// AppendPathRewrite preserves the input shell script and appends a small
// post-eval PATH rewrite. That keeps direnv/nix responsible for shell syntax:
// once their script has updated PATH, sandix stores that devshell PATH for
// sandboxed children and derives the interactive PATH from it.
func AppendPathRewrite(input []byte, baseline, mountPoint, sandixPath, trustedPath string) []byte {
	if len(input) == 0 {
		return input
	}

	var out strings.Builder
	out.Grow(len(input) + 256 + len(baseline) + len(mountPoint) + len(sandixPath) + len(trustedPath))
	out.Write(input)
	if len(input) > 0 && input[len(input)-1] != '\n' {
		out.WriteByte('\n')
	}
	out.WriteString("# sandix: rewrite PATH entries added by the environment script\n")
	out.WriteString("if [ -z \"${SANDIX_BASELINE_PATH+x}\" ]; then\n")
	out.WriteString("    SANDIX_BASELINE_PATH=")
	out.WriteString(shellQuote(baseline))
	out.WriteString("\n")
	out.WriteString("    export SANDIX_BASELINE_PATH\n")
	out.WriteString("fi\n")
	out.WriteString("SANDIX_PATH=\"$PATH\"\n")
	out.WriteString("export SANDIX_PATH\n")
	out.WriteString("PATH=\"$(")
	out.WriteString(shellQuote(sandixPath))
	out.WriteString(" rewrite --mount-point ")
	out.WriteString(shellQuote(mountPoint))
	out.WriteString(" --baseline \"$SANDIX_BASELINE_PATH\"")
	out.WriteString(" \"$SANDIX_PATH\")\"\n")
	if trustedPath != "" {
		out.WriteString("if [ -n \"$PATH\" ]; then\n")
		out.WriteString("    PATH=")
		out.WriteString(shellQuote(trustedPath))
		out.WriteString(":\"$PATH\"\n")
		out.WriteString("else\n")
		out.WriteString("    PATH=")
		out.WriteString(shellQuote(trustedPath))
		out.WriteString("\n")
		out.WriteString("fi\n")
	}
	out.WriteString("export PATH\n")
	return []byte(out.String())
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
