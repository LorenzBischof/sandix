package sandbox

import (
	"reflect"
	"testing"
)

func TestBaseEnvKeepsOnlySandboxInputs(t *testing.T) {
	got := BaseEnv(map[string]string{
		"HOME":      "/home/me",
		"PATH":      "/bin",
		"NIX_PATH":  "nixpkgs",
		"HOST_ONLY": "secret",
	})

	want := map[string]string{
		"HOME":     "/home/me",
		"PATH":     "/bin",
		"NIX_PATH": "nixpkgs",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BaseEnv() = %#v, want %#v", got, want)
	}
}

func TestLandrunArgsRendersEnvMountsAndCommandBoundary(t *testing.T) {
	got := landrunArgs(
		map[string]string{
			"PATH":    "/nix/store/tool/bin",
			"PROJECT": "value",
		},
		[]string{"--rox", "/nix/store", "--rwx", "/work"},
		[]string{"/nix/store/tool/bin/tool", "--flag"},
	)

	want := []string{
		"--env", "PROJECT=value",
		"--unrestricted-network",
		"--rox", "/nix/store",
		"--rwx", "/work",
		"--env", "HOME",
		"--env", "USER",
		"--env", "PATH=/nix/store/tool/bin",
		"--env", "TERM",
		"--env", "TERMINFO",
		"--env", "TERMINFO_DIRS",
		"--env", "COLORTERM",
		"--env", "LANG",
		"--env", "LC_ALL",
		"--env", "LC_CTYPE",
		"--env", "NIX_REMOTE",
		"--env", "SSL_CERT_FILE",
		"--env", "NIX_SSL_CERT_FILE",
		"--env", "NIX_PATH",
		"--",
		"/nix/store/tool/bin/tool", "--flag",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("landrunArgs() = %#v, want %#v", got, want)
	}
}
