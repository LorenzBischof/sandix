package nixwrap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const nixStorePrefix = "/nix/store/"
const officialNixCache = "https://cache.nixos.org/"
const officialNixCachePublicKey = "cache.nixos.org-1:6NCHdD59X431o0gWypbMrAURkbJ16ZPMQFGspcDShjY="
const officialNixCacheKeyName = "cache.nixos.org-1:"

var versionSuffixPattern = regexp.MustCompile(`^(.+)-[0-9].*$`)
var buildWrappedBinDir = BuildWrappedBinDir

type buildSpec struct {
	BuildScript string `json:"buildScript"`
	BuilderPath string `json:"builderPath"`
	Name        string `json:"name"`
	ShellPath   string `json:"shellPath"`
}

type wrapperCache struct {
	Version   int               `json:"version"`
	SandixKey string            `json:"sandixKey"`
	Entries   map[string]string `json:"entries"`
	path      string
	dirty     bool
}

// RewritePath strips non-store PATH entries and replaces Nix store PATH
// entries with Nix-built wrapper derivations that execute the original
// binaries through sandix exec.
func RewritePath(pathValue, nixPath, sandixPath, builderPath, landrunPath, shellPath string, trustedPackageNames []string) (string, error) {
	return RewritePathAdded(pathValue, "", nixPath, sandixPath, builderPath, landrunPath, shellPath, trustedPackageNames)
}

// RewritePathAdded preserves inherited PATH entries, strips newly-added
// non-store entries, and wraps newly-added Nix store entries.
func RewritePathAdded(pathValue, previousPath, nixPath, sandixPath, builderPath, landrunPath, shellPath string, trustedPackageNames []string) (string, error) {
	trustedNames := trustedNameSet(trustedPackageNames)
	previousEntries := pathEntrySet(previousPath)
	cache := loadWrapperCache(wrapperCacheKey(sandixPath, landrunPath, shellPath))
	defer cache.save()

	entries := filepath.SplitList(pathValue)
	rewrittenEntries := make([]string, 0, len(entries))
	for _, entry := range entries {
		if _, ok := previousEntries[entry]; ok {
			rewrittenEntries = append(rewrittenEntries, entry)
			continue
		}
		if !strings.HasPrefix(entry, nixStorePrefix) {
			continue
		}
		if isTrustedPackageBinDir(entry, trustedNames, nixPath) {
			rewrittenEntries = append(rewrittenEntries, entry)
			continue
		}
		wrapped, err := wrappedBinDir(entry, cache, nixPath, sandixPath, builderPath, landrunPath, shellPath)
		if err != nil {
			return "", err
		}
		rewrittenEntries = append(rewrittenEntries, filepath.Join(wrapped, "bin"))
	}
	return strings.Join(rewrittenEntries, string(filepath.ListSeparator)), nil
}

func wrappedBinDir(entry string, cache *wrapperCache, nixPath, sandixPath, builderPath, landrunPath, shellPath string) (string, error) {
	if wrapped, ok := cache.lookup(entry); ok {
		return wrapped, nil
	}
	wrapped, err := buildWrappedBinDir(entry, nixPath, sandixPath, builderPath, landrunPath, shellPath)
	if err != nil {
		return "", err
	}
	cache.store(entry, wrapped)
	return wrapped, nil
}

func pathEntrySet(pathValue string) map[string]struct{} {
	if pathValue == "" {
		return nil
	}
	entries := filepath.SplitList(pathValue)
	set := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		set[entry] = struct{}{}
	}
	return set
}

func wrapperCacheKey(sandixPath, landrunPath, shellPath string) string {
	hash := sha256.New()
	for _, value := range []string{sandixPath, landrunPath, shellPath} {
		hash.Write([]byte(value))
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func loadWrapperCache(sandixKey string) *wrapperCache {
	cache := &wrapperCache{
		Version:   1,
		SandixKey: sandixKey,
		Entries:   map[string]string{},
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return cache
	}
	cache.path = filepath.Join(cacheDir, "sandix", "nixwrap-cache.json")

	content, err := os.ReadFile(cache.path)
	if err != nil {
		return cache
	}

	var loaded wrapperCache
	if err := json.Unmarshal(content, &loaded); err != nil {
		return cache
	}
	if loaded.Version != cache.Version || loaded.SandixKey != sandixKey || loaded.Entries == nil {
		return cache
	}

	loaded.path = cache.path
	return &loaded
}

func (cache *wrapperCache) lookup(binDir string) (string, bool) {
	wrapped, ok := cache.Entries[binDir]
	if !ok {
		return "", false
	}
	if !pathExists(wrapped) {
		delete(cache.Entries, binDir)
		cache.dirty = true
		return "", false
	}
	return wrapped, true
}

func (cache *wrapperCache) store(binDir, wrapped string) {
	if cache.Entries == nil {
		cache.Entries = map[string]string{}
	}
	if cache.Entries[binDir] == wrapped {
		return
	}
	cache.Entries[binDir] = wrapped
	cache.dirty = true
}

func (cache *wrapperCache) save() {
	if !cache.dirty || cache.path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(cache.path), 0700); err != nil {
		return
	}

	content, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(cache.path), ".nixwrap-cache-*.json")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return
	}
	if err := os.Rename(tmpName, cache.path); err != nil {
		os.Remove(tmpName)
	}
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func trustedNameSet(names []string) map[string]struct{} {
	trusted := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name != "" {
			trusted[name] = struct{}{}
		}
	}
	return trusted
}

func isTrustedPackageBinDir(binDir string, trustedNames map[string]struct{}, nixPath string) bool {
	if len(trustedNames) == 0 {
		return false
	}
	storePath, ok := storePathForBinDir(binDir)
	if !ok {
		return false
	}
	if _, ok := trustedNames[packageNameFromStorePath(storePath)]; !ok {
		return false
	}
	return verifyOfficialCacheSignature(nixPath, storePath) == nil
}

func storePathForBinDir(binDir string) (string, bool) {
	cleaned := filepath.Clean(binDir)
	if filepath.Base(cleaned) != "bin" {
		return "", false
	}
	storePath := filepath.Dir(cleaned)
	if !strings.HasPrefix(storePath, nixStorePrefix) {
		return "", false
	}
	if filepath.Dir(storePath) != strings.TrimSuffix(nixStorePrefix, "/") {
		return "", false
	}
	return storePath, true
}

func packageNameFromStorePath(storePath string) string {
	name := filepath.Base(storePath)
	if len(name) > 33 && name[32] == '-' {
		name = name[33:]
	}
	if match := versionSuffixPattern.FindStringSubmatch(name); match != nil {
		return match[1]
	}
	return name
}

func verifyOfficialCacheSignature(nixPath, storePath string) error {
	infoCmd := exec.Command(
		nixPath,
		"path-info",
		"--store", officialNixCache,
		"--json",
		"--json-format", "1",
		storePath,
	)
	infoCmd.Stderr = os.Stderr
	infoOutput, err := infoCmd.Output()
	if err != nil {
		return err
	}

	var info map[string]struct {
		Signatures []string `json:"signatures"`
	}
	if err := json.Unmarshal(infoOutput, &info); err != nil {
		return err
	}
	if !hasOfficialCacheSignature(info[storePath].Signatures) {
		return fmt.Errorf("%s is not signed by %s", storePath, officialNixCacheKeyName)
	}

	cmd := exec.Command(
		nixPath,
		"store", "verify",
		"--store", officialNixCache,
		"--no-contents",
		"--sigs-needed", "1",
		"--option", "trusted-public-keys", officialNixCachePublicKey,
		storePath,
	)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func hasOfficialCacheSignature(signatures []string) bool {
	for _, signature := range signatures {
		if strings.HasPrefix(signature, officialNixCacheKeyName) {
			return true
		}
	}
	return false
}

// BuildWrappedBinDir asks the system Nix to build a wrapper derivation for one
// existing /nix/store/.../bin directory.
func BuildWrappedBinDir(binDir, nixPath, sandixPath, builderPath, landrunPath, shellPath string) (string, error) {
	buildScript, err := buildScript(binDir, sandixPath, landrunPath, shellPath)
	if err != nil {
		return "", err
	}

	specJSON, err := json.Marshal(buildSpec{
		BuildScript: buildScript,
		BuilderPath: builderPath,
		Name:        wrapperName(binDir),
		ShellPath:   shellPath,
	})
	if err != nil {
		return "", err
	}

	cmd := exec.Command(
		nixPath,
		"build",
		"--extra-experimental-features", "nix-command",
		"--option", "substitute", "false",
		"--impure",
		"--no-link",
		"--print-out-paths",
		"--expr", wrapperExpression(string(specJSON)),
	)
	cmd.Stderr = os.Stderr
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to build wrapper for %s: %w", binDir, err)
	}

	out := strings.TrimSpace(string(output))
	if first, _, ok := strings.Cut(out, "\n"); ok {
		out = first
	}
	if out == "" {
		return "", fmt.Errorf("nix returned an empty wrapper path for %s", binDir)
	}
	return out, nil
}

func wrapperExpression(specJSON string) string {
	return fmt.Sprintf(`
let
  spec = builtins.fromJSON %s;
in
derivation {
  name = spec.name;
  system = builtins.currentSystem;
  builder = toString (builtins.storePath (/. + spec.builderPath));
  WRAPPER_SHELL = toString (builtins.storePath (/. + spec.shellPath));
  BUILD_SCRIPT = spec.buildScript;
  args = [ ];
}
`, strconv.Quote(specJSON))
}

func wrapperName(binDir string) string {
	storePath := strings.TrimSuffix(binDir, "/")
	if filepath.Base(storePath) == "bin" {
		storePath = filepath.Dir(storePath)
	}
	name := filepath.Base(storePath)
	if len(name) > 33 && name[32] == '-' {
		name = name[33:]
	}
	if name == "." || name == "/" || name == "" {
		name = "path"
	}
	return name
}

func buildScript(binDir, sandixPath, landrunPath, shellPath string) (string, error) {
	entries, err := os.ReadDir(binDir)
	if err != nil {
		return "", err
	}

	var out strings.Builder
	out.WriteString("mkdir -p \"$out/bin\"\n")
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return "", err
		}
		if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		name := entry.Name()
		target := filepath.Join(binDir, name)
		out.WriteString("cat > ")
		out.WriteString("\"$out/bin\"/")
		out.WriteString(shellQuote(name))
		out.WriteString(" <<'SANDIX_WRAPPER_EOF'\n")
		out.WriteString("#!")
		out.WriteString(shellPath)
		out.WriteString("\n")
		out.WriteString("exec ")
		out.WriteString(shellQuote(sandixPath))
		out.WriteString(" exec --landrun ")
		out.WriteString(shellQuote(landrunPath))
		out.WriteString(" ")
		out.WriteString(shellQuote(target))
		out.WriteString(" \"$@\"\n")
		out.WriteString("SANDIX_WRAPPER_EOF\n")
		out.WriteString("chmod +x ")
		out.WriteString("\"$out/bin\"/")
		out.WriteString(shellQuote(name))
		out.WriteString("\n")
	}
	return out.String(), nil
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
