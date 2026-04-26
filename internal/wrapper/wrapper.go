package wrapper

import "fmt"

// Generate produces a landrun wrapper shell script that sandboxes the given
// binary. The script is a pure function of its inputs — same arguments always
// produce the same output.
func Generate(storeName, binName, landrunPath string) []byte {
	script := fmt.Sprintf(`#!/bin/sh
exec %s \
    $(env | cut -d= -f1 | while IFS= read -r env_name; do
        printf '%%s\n%%s\n' --env "$env_name"
    done) \
    --rox /nix/store \
    --rwx "$PWD" \
    --rw /tmp \
    --rw /dev \
    -- /nix/store/%s/bin/%s "$@"
`, landrunPath, storeName, binName)
	return []byte(script)
}
