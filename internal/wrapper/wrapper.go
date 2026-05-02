package wrapper

import "fmt"

// Generate produces a sandbox-exec wrapper shell script that sandboxes the given
// binary. The script is a pure function of its inputs — same arguments always
// produce the same output.
func Generate(storeName, binName, sandboxExecPath string) []byte {
	script := fmt.Sprintf(`#!/bin/sh
if [ -n "${SANDIX_PATH:-}" ]; then
    PATH="$SANDIX_PATH"
    export PATH
fi
exec %s /nix/store/%s/bin/%s "$@"
`, sandboxExecPath, storeName, binName)
	return []byte(script)
}
