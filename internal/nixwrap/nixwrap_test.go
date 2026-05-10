package nixwrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildScriptCreatesWrappers(t *testing.T) {
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "tool"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	got, err := buildScript(binDir, "/bin/sandix", "/nix/store/landrun/bin/landrun", "/nix/store/shell/bin/sh")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got, "#!/nix/store/shell/bin/sh\n") {
		t.Fatalf("build script should use the trusted store shell in generated wrappers: %q", got)
	}
	if !strings.Contains(got, "mkdir -p \"$out/bin\"") {
		t.Fatalf("build script should create output bin dir: %q", got)
	}
	if !strings.Contains(got, "exec '/bin/sandix' exec --landrun '/nix/store/landrun/bin/landrun' '"+binDir+"/tool' \"$@\"") {
		t.Fatalf("build script should wrap the real tool: %q", got)
	}
}

func TestWrapperExpressionUsesTrustedStoreShellBuilder(t *testing.T) {
	got := wrapperExpression(`{"buildScript":"true","builderPath":"/nix/store/builder/bin/sandix-wrapper-builder","name":"tool","shellPath":"/nix/store/shell/bin/sh"}`)

	if !strings.Contains(got, `builder = toString (builtins.storePath (/. + spec.builderPath));`) {
		t.Fatalf("wrapper expression should use the trusted wrapper builder: %q", got)
	}
	if strings.Contains(got, `PATH =`) {
		t.Fatalf("wrapper expression should not set a builder PATH: %q", got)
	}
	if !strings.Contains(got, `WRAPPER_SHELL = toString (builtins.storePath (/. + spec.shellPath));`) {
		t.Fatalf("wrapper expression should keep wrapper shell as an explicit derivation input: %q", got)
	}
	if strings.Contains(got, `builder = "/bin/sh";`) {
		t.Fatalf("wrapper expression should not use /bin/sh as the builder: %q", got)
	}
}

func TestWrapperNameUsesPackageStoreName(t *testing.T) {
	got := wrapperName("/nix/store/3pcn0admdqrrd0k405y9hyad4579wgb3-hello-2.12.3/bin")
	if got != "hello-2.12.3" {
		t.Fatalf("wrapperName() = %q", got)
	}
}

func TestRewritePathUsesCachedWrapper(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	binDir := "/nix/store/11111111111111111111111111111111-tool-1.0/bin"
	wrapped := t.TempDir()
	cacheKey := wrapperCacheKey("/bin/sandix", "/bin/landrun", "/bin/sh")
	cache := loadWrapperCache(cacheKey)
	cache.store(binDir, wrapped)
	cache.save()

	originalBuild := buildWrappedBinDir
	buildWrappedBinDir = func(binDir, nixPath, sandixPath, builderPath, landrunPath, shellPath string) (string, error) {
		t.Fatalf("BuildWrappedBinDir should not be called on cache hit")
		return "", nil
	}
	defer func() {
		buildWrappedBinDir = originalBuild
	}()

	got, err := RewritePath(binDir+":/usr/bin", "/bin/nix", "/bin/sandix", "/bin/builder", "/bin/landrun", "/bin/sh", nil)
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(wrapped, "bin")
	if got != want {
		t.Fatalf("RewritePath() = %q, want %q", got, want)
	}
}

func TestRewritePathStripsNewNonStoreEntries(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	binDir := "/nix/store/11111111111111111111111111111111-tool-1.0/bin"
	wrapped := t.TempDir()
	cacheKey := wrapperCacheKey("/bin/sandix", "/bin/landrun", "/bin/sh")
	cache := loadWrapperCache(cacheKey)
	cache.store(binDir, wrapped)
	cache.save()

	got, err := RewritePath("/tmp/project/bin:"+binDir+":/usr/bin", "/bin/nix", "/bin/sandix", "/bin/builder", "/bin/landrun", "/bin/sh", nil)
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(wrapped, "bin")
	if got != want {
		t.Fatalf("RewritePath() = %q, want %q", got, want)
	}
}

func TestRewritePathAddedKeepsInheritedProfileEntries(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	inheritedProfileBin := "/etc/profiles/per-user/me/bin"
	addedBin := "/nix/store/11111111111111111111111111111111-tool-1.0/bin"
	wrapped := t.TempDir()
	cacheKey := wrapperCacheKey("/bin/sandix", "/bin/landrun", "/bin/sh")
	cache := loadWrapperCache(cacheKey)
	cache.store(addedBin, wrapped)
	cache.save()

	got, err := RewritePathAdded(
		addedBin+":"+inheritedProfileBin+":/tmp/project/bin",
		inheritedProfileBin,
		"/bin/nix",
		"/bin/sandix",
		"/bin/builder",
		"/bin/landrun",
		"/bin/sh",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(wrapped, "bin") + ":" + inheritedProfileBin
	if got != want {
		t.Fatalf("RewritePathAdded() = %q, want %q", got, want)
	}
}

func TestRewritePathAddedKeepsInheritedStoreEntriesUnwrapped(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	inheritedBin := "/nix/store/00000000000000000000000000000000-jq-1.0/bin"
	addedBin := "/nix/store/11111111111111111111111111111111-tool-1.0/bin"
	wrapped := t.TempDir()
	cacheKey := wrapperCacheKey("/bin/sandix", "/bin/landrun", "/bin/sh")
	cache := loadWrapperCache(cacheKey)
	cache.store(addedBin, wrapped)
	cache.save()

	got, err := RewritePathAdded(
		addedBin+":"+inheritedBin+":/usr/bin",
		inheritedBin+":/usr/bin",
		"/bin/nix",
		"/bin/sandix",
		"/bin/builder",
		"/bin/landrun",
		"/bin/sh",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(wrapped, "bin") + ":" + inheritedBin + ":/usr/bin"
	if got != want {
		t.Fatalf("RewritePathAdded() = %q, want %q", got, want)
	}
}

func TestRewritePathAddedDoesNotBuildInheritedStoreEntries(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	inheritedBin := "/nix/store/00000000000000000000000000000000-jq-1.0/bin"

	originalBuild := buildWrappedBinDir
	buildWrappedBinDir = func(binDir, nixPath, sandixPath, builderPath, landrunPath, shellPath string) (string, error) {
		t.Fatalf("BuildWrappedBinDir should not be called for inherited store entry %q", binDir)
		return "", nil
	}
	defer func() {
		buildWrappedBinDir = originalBuild
	}()

	got, err := RewritePathAdded(inheritedBin+":/usr/bin", inheritedBin+":/usr/bin", "/bin/nix", "/bin/sandix", "/bin/builder", "/bin/landrun", "/bin/sh", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := inheritedBin + ":/usr/bin"
	if got != want {
		t.Fatalf("RewritePathAdded() = %q, want %q", got, want)
	}
}

func TestWrapperCacheInvalidatesWhenSandixKeyChanges(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	binDir := "/nix/store/11111111111111111111111111111111-tool-1.0/bin"
	wrapped := t.TempDir()
	oldKey := wrapperCacheKey("/bin/sandix-old", "/bin/landrun", "/bin/sh")
	cache := loadWrapperCache(oldKey)
	cache.store(binDir, wrapped)
	cache.save()

	newKey := wrapperCacheKey("/bin/sandix-new", "/bin/landrun", "/bin/sh")
	cache = loadWrapperCache(newKey)
	if _, ok := cache.lookup(binDir); ok {
		t.Fatalf("cache entry should not survive a sandix key change")
	}
}

func TestWrapperCacheKeyChangesWhenRuntimeInputsChange(t *testing.T) {
	base := wrapperCacheKey("/bin/sandix", "/bin/landrun", "/bin/sh")
	tests := map[string]string{
		"sandix":  wrapperCacheKey("/bin/sandix-new", "/bin/landrun", "/bin/sh"),
		"landrun": wrapperCacheKey("/bin/sandix", "/bin/landrun-new", "/bin/sh"),
		"shell":   wrapperCacheKey("/bin/sandix", "/bin/landrun", "/bin/sh-new"),
	}

	for name, got := range tests {
		if got == base {
			t.Fatalf("%s change should alter the cache key", name)
		}
	}
}

func TestWrapperCacheDropsMissingWrapperOutput(t *testing.T) {
	cache := &wrapperCache{
		Version:   1,
		SandixKey: "key",
		Entries: map[string]string{
			"/nix/store/11111111111111111111111111111111-tool-1.0/bin": filepath.Join(t.TempDir(), "missing"),
		},
	}

	if _, ok := cache.lookup("/nix/store/11111111111111111111111111111111-tool-1.0/bin"); ok {
		t.Fatalf("cache should not use a missing wrapper output")
	}
	if !cache.dirty {
		t.Fatalf("cache should be marked dirty after dropping a missing wrapper output")
	}
}

func TestRewritePathPreservesDirenvUnloadShape(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	parentBin := "/nix/store/11111111111111111111111111111111-parent-1.0/bin"
	nestedBin := "/nix/store/22222222222222222222222222222222-nested-1.0/bin"
	hostBin := "/etc/profiles/per-user/me/bin"

	parentWrapped := t.TempDir()
	nestedWrapped := t.TempDir()
	cache := loadWrapperCache(wrapperCacheKey("/bin/sandix", "/bin/landrun", "/bin/sh"))
	cache.store(parentBin, parentWrapped)
	cache.store(nestedBin, nestedWrapped)
	cache.save()

	hostPath := hostBin + ":/usr/bin"
	parentPath := parentBin + ":" + hostPath
	nestedPath := nestedBin + ":" + filepath.Join(parentWrapped, "bin") + ":" + hostPath
	unloadedPath := parentPath

	gotParent, err := RewritePathAdded(parentPath, hostPath, "/bin/nix", "/bin/sandix", "/bin/builder", "/bin/landrun", "/bin/sh", nil)
	if err != nil {
		t.Fatal(err)
	}
	gotNested, err := RewritePathAdded(nestedPath, gotParent, "/bin/nix", "/bin/sandix", "/bin/builder", "/bin/landrun", "/bin/sh", nil)
	if err != nil {
		t.Fatal(err)
	}
	gotUnloaded, err := RewritePathAdded(unloadedPath, gotNested, "/bin/nix", "/bin/sandix", "/bin/builder", "/bin/landrun", "/bin/sh", nil)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(gotUnloaded, filepath.Join(nestedWrapped, "bin")) {
		t.Fatalf("unloaded PATH should not contain nested wrapper: %q", gotUnloaded)
	}
	if !strings.Contains(gotUnloaded, filepath.Join(parentWrapped, "bin")) {
		t.Fatalf("unloaded PATH should keep parent wrapper: %q", gotUnloaded)
	}
	if gotParent != gotUnloaded {
		t.Fatalf("unloaded PATH should return to parent PATH\nparent:   %q\nunloaded: %q", gotParent, gotUnloaded)
	}
	wantNestedPrefix := filepath.Join(nestedWrapped, "bin") + ":" + filepath.Join(parentWrapped, "bin")
	if !strings.HasPrefix(gotNested, wantNestedPrefix) {
		t.Fatalf("nested PATH should preserve nested-before-parent order: %q", gotNested)
	}
}

func TestPackageNameFromStorePathStripsHashAndVersion(t *testing.T) {
	tests := map[string]string{
		"/nix/store/33333333333333333333333333333333-coreutils-9.10": "coreutils",
		"/nix/store/33333333333333333333333333333333-gnugrep-3.12":   "gnugrep",
		"/nix/store/33333333333333333333333333333333-tool":           "tool",
		"/tmp/not-store/tool-1.0":                                    "tool",
	}

	for path, want := range tests {
		if got := packageNameFromStorePath(path); got != want {
			t.Fatalf("packageNameFromStorePath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestStorePathForBinDirRequiresDirectStoreBin(t *testing.T) {
	got, ok := storePathForBinDir("/nix/store/33333333333333333333333333333333-coreutils-9.10/bin")
	if !ok || got != "/nix/store/33333333333333333333333333333333-coreutils-9.10" {
		t.Fatalf("storePathForBinDir() = %q, %v", got, ok)
	}

	if _, ok := storePathForBinDir("/nix/store/33333333333333333333333333333333-coreutils-9.10/libexec/bin"); ok {
		t.Fatalf("nested bin dir should not be treated as a package output bin dir")
	}
}

func TestHasOfficialCacheSignature(t *testing.T) {
	if !hasOfficialCacheSignature([]string{"cache.nixos.org-1:signature"}) {
		t.Fatalf("expected cache.nixos.org signature to be accepted")
	}
	if hasOfficialCacheSignature([]string{"other-cache:signature"}) {
		t.Fatalf("unexpectedly accepted non-official cache signature")
	}
}
