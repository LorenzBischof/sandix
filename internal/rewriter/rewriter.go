package rewriter

import (
	"strings"
)

// AppendPathRewrite preserves the input shell script and appends a small
// post-eval rewrite. direnv remains responsible for environment syntax;
// sandix rewrites PATH and keeps DIRENV_DIFF consistent with the result.
func AppendPathRewrite(input []byte, sandixPath, trustedPackageNames string) []byte {
	if len(input) == 0 {
		return input
	}

	var out strings.Builder
	out.Grow(len(input) + 256 + len(sandixPath) + len(trustedPackageNames))
	out.Write(input)
	if len(input) > 0 && input[len(input)-1] != '\n' {
		out.WriteByte('\n')
	}
	out.WriteString("# sandix: rewrite direnv PATH entries through Nix-built sandbox wrappers\n")
	out.WriteString("__sandix_rewrite=\"$(")
	out.WriteString(shellQuote(sandixPath))
	out.WriteString(" rewrite-direnv")
	if trustedPackageNames != "" {
		out.WriteString(" --trusted-package-names ")
		out.WriteString(shellQuote(trustedPackageNames))
	}
	out.WriteString(")\"\n")
	out.WriteString("__sandix_status=$?\n")
	out.WriteString("if [ \"$__sandix_status\" -ne 0 ]; then return \"$__sandix_status\"; fi\n")
	out.WriteString("eval \"$__sandix_rewrite\"\n")
	out.WriteString("unset __sandix_rewrite __sandix_status\n")
	return []byte(out.String())
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
