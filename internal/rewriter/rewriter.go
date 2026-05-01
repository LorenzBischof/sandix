package rewriter

import (
	"strings"
)

const nixStorePrefix = "/nix/store/"

// RewritePath rewrites /nix/store entries that were added after baseline.
// Entries already present in baseline are treated as trusted shell state and
// are preserved.
func RewritePath(pathValue, baseline, mountPoint string) string {
	trusted := pathEntrySet(baseline)
	entries := strings.Split(pathValue, ":")
	for i, entry := range entries {
		if trusted[entry] {
			continue
		}
		entries[i] = rewritePathEntry(entry, mountPoint)
	}
	return strings.Join(entries, ":")
}

func rewritePathEntry(entry, mountPoint string) string {
	if !strings.HasPrefix(entry, nixStorePrefix) {
		return entry
	}
	return strings.TrimRight(mountPoint, "/") + "/" + strings.TrimPrefix(entry, nixStorePrefix)
}

func pathEntrySet(pathValue string) map[string]bool {
	entries := strings.Split(pathValue, ":")
	set := make(map[string]bool, len(entries))
	for _, entry := range entries {
		set[entry] = true
	}
	return set
}

// AppendPathRewrite preserves the input shell script and appends a small
// post-eval PATH rewrite. That keeps direnv/nix responsible for shell syntax:
// once their script has updated PATH, sandix rewrites only newly-added entries.
func AppendPathRewrite(input []byte, baseline, mountPoint, sandixPath string) []byte {
	var out strings.Builder
	out.Grow(len(input) + 256 + len(baseline) + len(mountPoint) + len(sandixPath))
	out.Write(input)
	if len(input) > 0 && input[len(input)-1] != '\n' {
		out.WriteByte('\n')
	}
	out.WriteString("# sandix: rewrite PATH entries added by the environment script\n")
	out.WriteString("PATH=\"$(")
	out.WriteString(shellQuote(sandixPath))
	out.WriteString(" rewrite --mount-point ")
	out.WriteString(shellQuote(mountPoint))
	out.WriteString(" --baseline ")
	out.WriteString(shellQuote(baseline))
	out.WriteString(" \"$PATH\")\"\n")
	out.WriteString("export PATH\n")
	return []byte(out.String())
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
