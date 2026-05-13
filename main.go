package main

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lorenzbischof/sandix/internal/nixwrap"
	"github.com/lorenzbischof/sandix/internal/rewriter"
	"github.com/lorenzbischof/sandix/internal/sandbox"
)

func currentSandixPath() (string, error) {
	if os.Args[0] != "" {
		if strings.ContainsRune(os.Args[0], filepath.Separator) {
			return filepath.Abs(os.Args[0])
		}
		if path, err := exec.LookPath(os.Args[0]); err == nil {
			return path, nil
		}
	}
	return os.Executable()
}

func currentShellPath() (string, error) {
	shellPath, err := exec.LookPath("sh")
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(shellPath); err == nil {
		shellPath = resolved
	}
	return shellPath, nil
}

type trustedFlags struct {
	bashPath            *string
	builderPath         *string
	direnvPath          *string
	landrunPath         *string
	nixPath             *string
	sandixPath          *string
	shellPath           *string
	trustedPackageNames *string
}

func addTrustedFlags(fs *flag.FlagSet) trustedFlags {
	return trustedFlags{
		bashPath:            fs.String("bash", "", "trusted bash executable used to evaluate direnv .envrc files"),
		builderPath:         fs.String("builder", "", "trusted Nix derivation builder used to create command wrappers"),
		direnvPath:          fs.String("direnv", "direnv", "trusted direnv executable"),
		landrunPath:         fs.String("landrun", "", "trusted landrun executable"),
		nixPath:             fs.String("nix", "nix", "trusted system nix executable"),
		sandixPath:          fs.String("sandix", "", "trusted sandix executable used by generated command wrappers"),
		shellPath:           fs.String("shell", "", "trusted shell used by Nix wrapper builders and generated wrappers"),
		trustedPackageNames: fs.String("trusted-package-names", "", "comma-separated package names to leave unwrapped when signed by cache.nixos.org"),
	}
}

func (f trustedFlags) require(name string, value *string) {
	if *value == "" {
		log.Fatalf("--%s is required", name)
	}
}

func (f trustedFlags) resolvedShellPath() string {
	if *f.shellPath != "" {
		return *f.shellPath
	}
	shellPath, err := currentShellPath()
	if err != nil {
		log.Fatalf("Failed to resolve shell path: %v", err)
	}
	return shellPath
}

func (f trustedFlags) resolvedSandixPath() string {
	if *f.sandixPath != "" {
		return *f.sandixPath
	}
	sandixPath, err := currentSandixPath()
	if err != nil {
		log.Fatalf("Failed to resolve sandix path: %v", err)
	}
	return sandixPath
}

func (f trustedFlags) resolvedBashPath() string {
	if *f.bashPath != "" {
		return *f.bashPath
	}
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		log.Fatalf("bash not found in PATH: %v", err)
	}
	return bashPath
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: sandix <command> [args]\n")
		fmt.Fprintf(os.Stderr, "Commands: rewrite-direnv, wrap-path, direnv-export, direnv-bash, exec\n")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "rewrite-direnv":
		cmdRewriteDirenv(os.Args[2:])
	case "wrap-path":
		cmdWrapPath(os.Args[2:])
	case "direnv-export":
		cmdDirenvExport(os.Args[2:])
	case "direnv-bash":
		cmdDirenvBash(os.Args[2:])
	case "exec":
		cmdExec(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

type envOverlay struct {
	Set   map[string]string `json:"set"`
	Unset []string          `json:"unset"`
}

type direnvDiff struct {
	Previous map[string]string `json:"p"`
	Next     map[string]string `json:"n"`
}

func cmdDirenvBash(args []string) {
	fs := flag.NewFlagSet("direnv-bash", flag.ExitOnError)
	trusted := addTrustedFlags(fs)
	fs.Parse(args)
	trusted.require("landrun", trusted.landrunPath)

	hostEnv := envMap(os.Environ())
	currentDiff, currentDiffOK, err := readDirenvDiff()
	if err != nil {
		log.Fatalf("failed to decode DIRENV_DIFF: %v", err)
	}
	evaluatorEnv := direnvEvaluatorEnv(hostEnv, currentDiff, currentDiffOK)

	bashPath := trusted.resolvedBashPath()

	landrunEnv := copyEnv(hostEnv)
	if currentDiffOK {
		for _, key := range direnvUnsetKeys(currentDiff) {
			delete(landrunEnv, key)
		}
	}
	cmd, err := sandbox.LandrunCommand(*trusted.landrunPath, landrunEnv, evaluatorEnv, append([]string{bashPath}, fs.Args()...))
	if err != nil {
		log.Fatalf("failed to build sandbox command: %v", err)
	}
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	output, err := cmd.Output()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			log.Fatalf("sandboxed direnv bash failed: %v", err)
		}
	}

	var sandboxResult map[string]string
	if jsonErr := json.Unmarshal(bytes.TrimSpace(output), &sandboxResult); jsonErr != nil {
		_, _ = os.Stdout.Write(output)
		os.Exit(exitCode)
	}

	overlay := diffEnv(evaluatorEnv, sandboxResult)
	interactiveResult := applyOverlay(hostEnv, overlay)

	encoded, err := json.Marshal(interactiveResult)
	if err != nil {
		log.Fatalf("failed to encode direnv environment: %v", err)
	}
	fmt.Println(string(encoded))
	os.Exit(exitCode)
}

func direnvEvaluatorEnv(hostEnv map[string]string, diff direnvDiff, diffOK bool) map[string]string {
	input := sandbox.BaseEnv(hostEnv)
	if !diffOK {
		return input
	}
	for key, value := range diff.Previous {
		input[key] = value
	}
	return input
}

func diffEnv(before, after map[string]string) envOverlay {
	overlay := envOverlay{Set: make(map[string]string)}
	for key, beforeValue := range before {
		afterValue, ok := after[key]
		if !ok {
			overlay.Unset = append(overlay.Unset, key)
			continue
		}
		if afterValue != beforeValue {
			overlay.Set[key] = afterValue
		}
	}
	for key, afterValue := range after {
		if _, ok := before[key]; !ok {
			overlay.Set[key] = afterValue
		}
	}
	return overlay
}

func applyOverlay(base map[string]string, overlay envOverlay) map[string]string {
	result := copyEnv(base)
	for _, key := range overlay.Unset {
		delete(result, key)
	}
	for key, value := range overlay.Set {
		result[key] = value
	}
	return result
}

func copyEnv(env map[string]string) map[string]string {
	copied := make(map[string]string, len(env))
	for key, value := range env {
		copied[key] = value
	}
	return copied
}

func readDirenvDiff() (direnvDiff, bool, error) {
	encoded := os.Getenv("DIRENV_DIFF")
	if encoded == "" {
		return direnvDiff{}, false, nil
	}

	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		data, err = base64.URLEncoding.DecodeString(encoded)
	}
	if err != nil {
		return direnvDiff{}, true, err
	}

	reader, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return direnvDiff{}, true, err
	}
	defer reader.Close()

	decoded, err := io.ReadAll(reader)
	if err != nil {
		return direnvDiff{}, true, err
	}

	var diff direnvDiff
	if err := json.Unmarshal(decoded, &diff); err != nil {
		return direnvDiff{}, true, err
	}
	if diff.Previous == nil {
		diff.Previous = make(map[string]string)
	}
	if diff.Next == nil {
		diff.Next = make(map[string]string)
	}
	return diff, true, nil
}

func encodeDirenvDiff(diff direnvDiff) (string, error) {
	encodedJSON, err := json.Marshal(diff)
	if err != nil {
		return "", err
	}

	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	if _, err := writer.Write(encodedJSON); err != nil {
		writer.Close()
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(compressed.Bytes()), nil
}

func direnvUnsetKeys(diff direnvDiff) []string {
	keys := make([]string, 0)
	for key := range diff.Previous {
		if key == "PATH" {
			continue
		}
		if _, exists := diff.Next[key]; exists {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func envMap(environ []string) map[string]string {
	env := make(map[string]string, len(environ))
	for _, item := range environ {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		env[key] = value
	}
	return env
}

func cmdDirenvExport(args []string) {
	fs := flag.NewFlagSet("direnv-export", flag.ExitOnError)
	trusted := addTrustedFlags(fs)
	fs.Parse(args)
	if fs.NArg() == 0 {
		log.Fatalf("Usage: sandix direnv-export [trusted flags] SHELL")
	}

	cmd := exec.Command(*trusted.direnvPath, append([]string{"export"}, fs.Args()...)...)
	cmd.Stderr = os.Stderr
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			_, _ = os.Stdout.Write(exitErr.Stderr)
			os.Exit(exitErr.ExitCode())
		}
		log.Fatalf("direnv export failed: %v", err)
	}

	sandixPath := trusted.resolvedSandixPath()
	rewritten := rewriter.AppendPathRewrite(output, sandixPath, *trusted.trustedPackageNames)
	os.Stdout.Write(rewritten)
}

func cmdRewriteDirenv(args []string) {
	fs := flag.NewFlagSet("rewrite-direnv", flag.ExitOnError)
	trusted := addTrustedFlags(fs)
	fs.Parse(args)
	if fs.NArg() != 0 {
		log.Fatalf("Usage: sandix rewrite-direnv [trusted flags]")
	}

	diff, ok, err := readDirenvDiff()
	if err != nil {
		log.Fatalf("failed to decode DIRENV_DIFF: %v", err)
	}
	if !ok || !direnvDiffIsActive(diff) {
		return
	}

	rewrittenPath, err := rewriteDirenvPath(diff, true, trusted)
	if err != nil {
		log.Fatalf("failed to rewrite PATH through Nix wrappers: %v", err)
	}

	fmt.Printf("PATH=%s\n", shellQuote(rewrittenPath))
	fmt.Print("export PATH\n")
	diff.Next["PATH"] = rewrittenPath
	encoded, err := encodeDirenvDiff(diff)
	if err != nil {
		log.Fatalf("failed to encode DIRENV_DIFF: %v", err)
	}
	fmt.Printf("DIRENV_DIFF=%s\n", shellQuote(encoded))
	fmt.Print("export DIRENV_DIFF\n")
}

func direnvDiffIsActive(diff direnvDiff) bool {
	_, ok := diff.Next["DIRENV_DIR"]
	return ok
}

func cmdWrapPath(args []string) {
	fs := flag.NewFlagSet("wrap-path", flag.ExitOnError)
	trusted := addTrustedFlags(fs)
	fs.Parse(args)
	if fs.NArg() != 0 {
		log.Fatalf("Usage: sandix wrap-path [trusted flags]")
	}

	diff, ok, err := readDirenvDiff()
	if err != nil {
		log.Fatalf("failed to decode DIRENV_DIFF: %v", err)
	}
	rewritten, err := rewriteDirenvPath(diff, ok, trusted)
	if err != nil {
		log.Fatalf("failed to rewrite PATH through Nix wrappers: %v", err)
	}
	fmt.Print(rewritten)
}

func rewriteDirenvPath(diff direnvDiff, diffOK bool, trusted trustedFlags) (string, error) {
	pathValue := os.Getenv("PATH")
	previousPath := ""
	if diffOK {
		if direnvPath, exists := diff.Next["PATH"]; exists {
			pathValue = direnvPath
		}
		if direnvPath, exists := diff.Previous["PATH"]; exists {
			previousPath = direnvPath
		}
	}

	trusted.require("builder", trusted.builderPath)
	trusted.require("landrun", trusted.landrunPath)
	sandixPath := trusted.resolvedSandixPath()
	shellPath := trusted.resolvedShellPath()

	return nixwrap.RewritePathAdded(pathValue, previousPath, *trusted.nixPath, sandixPath, *trusted.builderPath, *trusted.landrunPath, shellPath, trustedPackageNamesFromFlag(*trusted.trustedPackageNames))
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func trustedPackageNamesFromFlag(value string) []string {
	var names []string
	for _, name := range strings.Split(value, ",") {
		name = strings.TrimSpace(name)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func cmdExec(args []string) {
	fs := flag.NewFlagSet("exec", flag.ExitOnError)
	trusted := addTrustedFlags(fs)
	fs.Parse(args)
	if fs.NArg() == 0 {
		log.Fatalf("Usage: sandix exec --landrun PATH COMMAND [ARGS...]")
	}
	trusted.require("landrun", trusted.landrunPath)

	landrunEnv := envMap(os.Environ())
	diff, ok, err := readDirenvDiff()
	if err != nil {
		log.Fatalf("failed to decode DIRENV_DIFF: %v", err)
	}

	commandEnv := sandbox.BaseEnv(landrunEnv)
	if ok {
		commandEnv = diff.Next

		for _, key := range direnvUnsetKeys(diff) {
			delete(landrunEnv, key)
		}
	}

	cmd, err := sandbox.LandrunCommand(*trusted.landrunPath, landrunEnv, commandEnv, fs.Args())
	if err != nil {
		log.Fatalf("failed to build sandbox command: %v", err)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		log.Fatalf("landrun failed: %v", err)
	}
}
