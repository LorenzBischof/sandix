package wrapper

import (
	"fmt"
	"strings"
)

// Generate produces a wrapper shell script that sandboxes the given
// binary. The script is a pure function of its inputs — same arguments always
// produce the same output.
func Generate(storeName, binName, sandixPath string) []byte {
	script := fmt.Sprintf(`#!/bin/sh
exec %s exec /nix/store/%s/bin/%s "$@"
`, shellQuote(sandixPath), storeName, binName)
	return []byte(script)
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
