package rewriter

import (
	"path/filepath"
	"strings"
)

const nixStorePrefix = "/nix/store/"

// RewritePath rewrites /nix/store entries to the sandboxed FUSE mount.
func RewritePath(pathValue, mountPoint string) string {
	entries := filepath.SplitList(pathValue)
	for i, entry := range entries {
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

// AppendPathRewrite preserves the input shell script and appends a small
// post-eval PATH rewrite. direnv remains responsible for environment syntax
// and state; sandix only rewrites the interactive PATH from DIRENV_DIFF.
func AppendPathRewrite(input []byte, mountPoint, sandixPath, trustedPath string) []byte {
	if len(input) == 0 {
		return input
	}

	var out strings.Builder
	out.Grow(len(input) + 256 + len(mountPoint) + len(sandixPath) + len(trustedPath))
	out.Write(input)
	if len(input) > 0 && input[len(input)-1] != '\n' {
		out.WriteByte('\n')
	}
	out.WriteString("# sandix: rewrite direnv PATH entries through the sandboxed store\n")
	out.WriteString("PATH=\"$(")
	out.WriteString(shellQuote(sandixPath))
	out.WriteString(" direnv-path --mount-point ")
	out.WriteString(shellQuote(mountPoint))
	out.WriteString(")\"\n")
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
