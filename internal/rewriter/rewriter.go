package rewriter

import "regexp"

// binDirRe matches /nix/store/<hash-name>/bin only when bin is the final path
// component — i.e., NOT followed by /. This targets PATH-style directory
// entries without touching file references like /nix/store/.../bin/bash.
var binDirRe = regexp.MustCompile(`/nix/store/[^/]+/bin(?:[^/]|$)`)

// Rewrite replaces /nix/store/<hash>/bin directory references with
// <mountPoint>/<hash>/bin throughout the input text.
func Rewrite(input []byte, mountPoint string) []byte {
	return binDirRe.ReplaceAllFunc(input, func(match []byte) []byte {
		// The match includes one trailing char (or is at EOF).
		// Replace only the /nix/store prefix, keep <hash>/bin and trailing char.
		// /nix/store/ is 11 bytes.
		result := make([]byte, 0, len(mountPoint)+1+len(match)-11)
		result = append(result, mountPoint...)
		result = append(result, '/')
		result = append(result, match[11:]...)
		return result
	})
}
