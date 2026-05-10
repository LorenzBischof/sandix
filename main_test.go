package main

import (
	"encoding/base64"
	"testing"
)

func TestDiffEnvComputesOverlayFromSandboxInput(t *testing.T) {
	before := map[string]string{
		"PATH": "/bin",
		"HOME": "/home/me",
		"KEEP": "same",
		"DROP": "value",
	}
	after := map[string]string{
		"PATH": "/nix/store/tool/bin:/bin",
		"HOME": "/home/me",
		"KEEP": "same",
		"NEW":  "value",
	}

	overlay := diffEnv(before, after)

	if overlay.Set["PATH"] != "/nix/store/tool/bin:/bin" {
		t.Fatalf("expected PATH overlay, got %q", overlay.Set["PATH"])
	}
	if overlay.Set["NEW"] != "value" {
		t.Fatalf("expected NEW overlay, got %q", overlay.Set["NEW"])
	}
	if len(overlay.Unset) != 1 || overlay.Unset[0] != "DROP" {
		t.Fatalf("expected DROP unset, got %#v", overlay.Unset)
	}
	if _, ok := overlay.Set["KEEP"]; ok {
		t.Fatalf("unchanged key should not be in overlay")
	}
}

func TestApplyOverlayPreservesHostOnlyVars(t *testing.T) {
	host := map[string]string{
		"HOST_ONLY": "keep",
		"PATH":      "/usr/bin",
		"DROP":      "host",
	}
	overlay := envOverlay{
		Set: map[string]string{
			"PATH": "/nix/store/tool/bin:/usr/bin",
			"NEW":  "value",
		},
		Unset: []string{"DROP"},
	}

	got := applyOverlay(host, overlay)

	if got["HOST_ONLY"] != "keep" {
		t.Fatalf("host-only variable was not preserved: %#v", got)
	}
	if got["PATH"] != "/nix/store/tool/bin:/usr/bin" {
		t.Fatalf("PATH overlay not applied: %#v", got)
	}
	if got["NEW"] != "value" {
		t.Fatalf("NEW overlay not applied: %#v", got)
	}
	if _, ok := got["DROP"]; ok {
		t.Fatalf("DROP should be unset: %#v", got)
	}
}

func TestBaseSandboxEnvUsesPath(t *testing.T) {
	got := baseSandboxEnv(map[string]string{
		"PATH":      "/devshell/bin",
		"HOST_ONLY": "not-forwarded",
		"HOME":      "/home/me",
	})

	if got["PATH"] != "/devshell/bin" {
		t.Fatalf("expected PATH in sandbox PATH, got %q", got["PATH"])
	}
	if _, ok := got["HOST_ONLY"]; ok {
		t.Fatalf("host-only variable should not be in reduced sandbox input")
	}
	if got["HOME"] != "/home/me" {
		t.Fatalf("HOME should be forwarded")
	}
}

func TestDirenvEvaluatorEnvUsesPreviousDirenvEnv(t *testing.T) {
	got := direnvEvaluatorEnv(
		map[string]string{
			"PATH":      "/run/user/1000/sandix/store/inner/bin:/bin",
			"HOST_ONLY": "not-forwarded",
		},
		direnvDiff{
			Previous: map[string]string{
				"PATH":         "/nix/store/parent/bin:/bin",
				"PROJECT_ONLY": "from-parent",
			},
			Next: map[string]string{
				"PATH": "/nix/store/inner/bin:/bin",
			},
		},
		true,
	)

	if got["PATH"] != "/nix/store/parent/bin:/bin" {
		t.Fatalf("expected previous direnv PATH in sandbox input, got %q", got["PATH"])
	}
	if _, ok := got["HOST_ONLY"]; ok {
		t.Fatalf("host-only variable should not be in reduced sandbox input")
	}
	if got["PROJECT_ONLY"] != "from-parent" {
		t.Fatalf("expected previous direnv variables in sandbox input, got %q", got["PROJECT_ONLY"])
	}
}

func TestDirenvUnsetKeysIncludesForwardedHostVars(t *testing.T) {
	got := direnvUnsetKeys(direnvDiff{
		Previous: map[string]string{
			"NIX_PATH":        "host",
			"NIX_REMOTE":      "daemon",
			"SSL_CERT_FILE":   "/host/certs",
			"PATH":            "/bin",
			"DIRENV_DIR":      "-/tmp/project",
			"HOST_REMOVED":    "old",
			"PROJECT_STAYS":   "value",
			"PROJECT_REMOVED": "value",
		},
		Next: map[string]string{
			"PROJECT_STAYS": "value",
		},
	})

	want := []string{"DIRENV_DIR", "HOST_REMOVED", "NIX_PATH", "NIX_REMOTE", "PROJECT_REMOVED", "SSL_CERT_FILE"}
	if len(got) != len(want) {
		t.Fatalf("expected removed keys %#v, got %#v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected removed keys %#v, got %#v", want, got)
		}
	}
}

func TestEncodeDirenvDiffRoundTripsThroughReader(t *testing.T) {
	want := direnvDiff{
		Previous: map[string]string{
			"PATH": "/nix/store/old/bin:/bin",
			"FOO":  "old",
		},
		Next: map[string]string{
			"PATH": "/nix/store/new-wrapper/bin:/nix/store/old/bin",
			"FOO":  "new",
		},
	}

	encoded, err := encodeDirenvDiff(want)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := base64.URLEncoding.DecodeString(encoded); err != nil {
		t.Fatalf("encoded DIRENV_DIFF should use padded URL base64: %v", err)
	}
	t.Setenv("DIRENV_DIFF", encoded)

	got, ok, err := readDirenvDiff()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("expected DIRENV_DIFF to be read")
	}
	if got.Previous["PATH"] != want.Previous["PATH"] || got.Next["PATH"] != want.Next["PATH"] || got.Next["FOO"] != want.Next["FOO"] {
		t.Fatalf("readDirenvDiff() = %#v, want %#v", got, want)
	}
}

func TestDirenvDiffIsActiveRequiresNextDirenvDir(t *testing.T) {
	if !direnvDiffIsActive(direnvDiff{Next: map[string]string{"DIRENV_DIR": "-/tmp/project"}}) {
		t.Fatalf("expected diff with next DIRENV_DIR to be active")
	}
	if direnvDiffIsActive(direnvDiff{Next: map[string]string{"PATH": "/usr/bin"}}) {
		t.Fatalf("expected diff without next DIRENV_DIR to be inactive")
	}
}
