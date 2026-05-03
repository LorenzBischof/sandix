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
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	gofuse "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	sandixfuse "github.com/lorenzbischof/sandix/internal/fuse"
	"github.com/lorenzbischof/sandix/internal/rewriter"
)

func defaultMountPoint() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "sandix", "store")
	}
	return filepath.Join("/run/user", strconv.Itoa(os.Getuid()), "sandix", "store")
}

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

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: sandix <command> [args]\n")
		fmt.Fprintf(os.Stderr, "Commands: fuse, wrap, rewrite, direnv-path, direnv-bash, exec\n")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "fuse":
		cmdFuse(os.Args[2:])
	case "wrap":
		cmdWrap(os.Args[2:])
	case "rewrite":
		cmdRewrite(os.Args[2:])
	case "direnv-path":
		cmdDirenvPath(os.Args[2:])
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
	hostEnv := envMap(os.Environ())
	currentDiff, currentDiffOK, err := readDirenvDiff()
	if err != nil {
		log.Fatalf("failed to decode DIRENV_DIFF: %v", err)
	}
	sandboxInput := direnvBashInputEnv(hostEnv, currentDiff, currentDiffOK)

	sandixPath, err := currentSandixPath()
	if err != nil {
		log.Fatalf("Failed to resolve sandix path: %v", err)
	}
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		log.Fatalf("bash not found in PATH: %v", err)
	}

	cmd := exec.Command(sandixPath, append([]string{"exec", bashPath}, args...)...)
	cmd.Env = append(envList(hostEnv), "SANDIX_DIRENV_EVAL=1")
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

	overlay := diffEnv(sandboxInput, sandboxResult)
	interactiveResult := applyOverlay(hostEnv, overlay)

	encoded, err := json.Marshal(interactiveResult)
	if err != nil {
		log.Fatalf("failed to encode direnv environment: %v", err)
	}
	fmt.Println(string(encoded))
	os.Exit(exitCode)
}

func sandboxInputEnv(hostEnv map[string]string) map[string]string {
	keys := []string{
		"HOME",
		"USER",
		"TERM",
		"TERMINFO",
		"TERMINFO_DIRS",
		"COLORTERM",
		"LANG",
		"LC_ALL",
		"LC_CTYPE",
		"NIX_REMOTE",
		"SSL_CERT_FILE",
		"NIX_SSL_CERT_FILE",
		"NIX_PATH",
	}

	reduced := make(map[string]string)
	for _, key := range keys {
		if value, ok := hostEnv[key]; ok {
			reduced[key] = value
		}
	}

	if value, ok := hostEnv["PATH"]; ok {
		reduced["PATH"] = value
	}

	return reduced
}

func direnvBashInputEnv(hostEnv map[string]string, diff direnvDiff, diffOK bool) map[string]string {
	input := sandboxInputEnv(hostEnv)
	if !diffOK {
		return input
	}
	if direnvPath, exists := diff.Previous["PATH"]; exists {
		input["PATH"] = direnvPath
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
	result := make(map[string]string, len(base)+len(overlay.Set))
	for key, value := range base {
		result[key] = value
	}
	for _, key := range overlay.Unset {
		delete(result, key)
	}
	for key, value := range overlay.Set {
		result[key] = value
	}
	return result
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

func removedDirenvKeys(diff direnvDiff) []string {
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

func envList(env map[string]string) []string {
	items := make([]string, 0, len(env))
	for key, value := range env {
		items = append(items, key+"="+value)
	}
	return items
}

func cmdFuse(args []string) {
	fs := flag.NewFlagSet("fuse", flag.ExitOnError)
	mountPoint := fs.String("mount-point", defaultMountPoint(), "FUSE mount point")
	debug := fs.Bool("debug", false, "enable FUSE debug logging")
	fs.Parse(args)

	if err := os.MkdirAll(*mountPoint, 0755); err != nil {
		log.Fatalf("Failed to create mount point %s: %v", *mountPoint, err)
	}

	sandixPath, err := currentSandixPath()
	if err != nil {
		log.Fatalf("Failed to resolve sandix path: %v", err)
	}

	root := &sandixfuse.RootNode{SandixPath: sandixPath}
	server, err := gofuse.Mount(*mountPoint, root, &gofuse.Options{
		MountOptions: fuse.MountOptions{
			Debug:      *debug,
			AllowOther: false,
			FsName:     "sandix",
			Name:       "sandix",
		},
	})
	if err != nil {
		log.Fatalf("Mount failed: %v", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		server.Unmount()
	}()

	log.Printf("Serving sandboxed store at %s", *mountPoint)
	server.Wait()
}

func cmdWrap(args []string) {
	fs := flag.NewFlagSet("wrap", flag.ExitOnError)
	mountPoint := fs.String("mount-point", defaultMountPoint(), "sandboxed store mount point")
	trustedPath := fs.String("trusted-path", "", "trusted host PATH entries to prepend outside the sandbox")
	fs.Parse(args)

	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Fatalf("Failed to read stdin: %v", err)
	}

	sandixPath, err := currentSandixPath()
	if err != nil {
		log.Fatalf("Failed to resolve sandix path: %v", err)
	}

	output := rewriter.AppendPathRewrite(input, *mountPoint, sandixPath, *trustedPath)
	os.Stdout.Write(output)
}

func cmdRewrite(args []string) {
	fs := flag.NewFlagSet("rewrite", flag.ExitOnError)
	mountPoint := fs.String("mount-point", defaultMountPoint(), "sandboxed store mount point")
	fs.Parse(args)

	pathValue := os.Getenv("PATH")
	switch fs.NArg() {
	case 0:
	case 1:
		pathValue = fs.Arg(0)
	default:
		log.Fatalf("Usage: sandix rewrite [--mount-point PATH] [PATH]")
	}

	fmt.Print(rewriter.RewritePath(pathValue, *mountPoint))
}

func cmdDirenvPath(args []string) {
	fs := flag.NewFlagSet("direnv-path", flag.ExitOnError)
	mountPoint := fs.String("mount-point", defaultMountPoint(), "sandboxed store mount point")
	fs.Parse(args)
	if fs.NArg() != 0 {
		log.Fatalf("Usage: sandix direnv-path [--mount-point PATH]")
	}

	diff, ok, err := readDirenvDiff()
	if err != nil {
		log.Fatalf("failed to decode DIRENV_DIFF: %v", err)
	}
	pathValue := os.Getenv("PATH")
	if ok {
		if direnvPath, exists := diff.Next["PATH"]; exists {
			pathValue = direnvPath
		}
	}

	fmt.Print(rewriter.RewritePath(pathValue, *mountPoint))
}

func cmdExec(args []string) {
	fs := flag.NewFlagSet("exec", flag.ExitOnError)
	fs.Parse(args)
	if fs.NArg() == 0 {
		log.Fatalf("Usage: sandix exec COMMAND [ARGS...]")
	}

	landrunPath, err := exec.LookPath("landrun")
	if err != nil {
		log.Fatalf("landrun not found in PATH: %v", err)
	}

	landrunEnv := envMap(os.Environ())
	landrunArgs := make([]string, 0, 64+fs.NArg())
	diff, ok, err := readDirenvDiff()
	if err != nil {
		log.Fatalf("failed to decode DIRENV_DIFF: %v", err)
	}

	commandPath := os.Getenv("PATH")
	commandEnv := map[string]string(nil)
	if ok {
		commandEnv = diff.Next
		if os.Getenv("SANDIX_DIRENV_EVAL") == "1" {
			commandEnv = diff.Previous
		}
		if direnvPath, exists := commandEnv["PATH"]; exists {
			commandPath = direnvPath
		}

		for _, key := range removedDirenvKeys(diff) {
			delete(landrunEnv, key)
		}

		keys := make([]string, 0, len(commandEnv))
		for key := range commandEnv {
			if key == "PATH" || isBaseSandboxEnvKey(key) {
				continue
			}
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			landrunArgs = append(landrunArgs, "--env", key+"="+commandEnv[key])
		}
	}
	delete(landrunEnv, "SANDIX_DIRENV_EVAL")

	addSandboxFilesystemArgs(&landrunArgs)
	addBaseSandboxEnvArgs(&landrunArgs, commandPath)

	landrunArgs = append(landrunArgs, "--")
	landrunArgs = append(landrunArgs, fs.Args()...)

	cmd := exec.Command(landrunPath, landrunArgs...)
	cmd.Env = envList(landrunEnv)
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

func addSandboxFilesystemArgs(args *[]string) {
	home := os.Getenv("HOME")
	if home != "" {
		xdgConfigHome := os.Getenv("XDG_CONFIG_HOME")
		if xdgConfigHome == "" {
			xdgConfigHome = filepath.Join(home, ".config")
		}
		xdgCacheHome := os.Getenv("XDG_CACHE_HOME")
		if xdgCacheHome == "" {
			xdgCacheHome = filepath.Join(home, ".cache")
		}
		if pathExists(filepath.Join(xdgConfigHome, "direnv")) {
			*args = append(*args, "--ro", filepath.Join(xdgConfigHome, "direnv"))
		}
		if pathExists(filepath.Join(xdgConfigHome, "nix")) {
			*args = append(*args, "--ro", filepath.Join(xdgConfigHome, "nix"))
		}
		if pathExists(xdgCacheHome) {
			*args = append(*args, "--rw", xdgCacheHome)
		}
	}

	for _, path := range []string{
		"/etc/nix",
		"/etc/resolv.conf",
		"/etc/hosts",
		"/etc/nsswitch.conf",
	} {
		if pathExists(path) {
			*args = append(*args, "--ro", path)
		}
	}
	if pathExists("/nix/var/nix/daemon-socket/socket") {
		*args = append(*args, "--rw", "/nix/var/nix/daemon-socket/socket")
	}

	pwd := os.Getenv("PWD")
	if pwd == "" {
		var err error
		pwd, err = os.Getwd()
		if err != nil {
			log.Fatalf("failed to resolve working directory: %v", err)
		}
	}

	*args = append(*args,
		"--unrestricted-network",
		"--rox", "/nix/store",
		"--rwx", pwd,
		"--rw", "/tmp",
		"--rw", "/proc,/dev,/sys",
	)
}

func addBaseSandboxEnvArgs(args *[]string, commandPath string) {
	for _, key := range baseSandboxEnvKeys {
		if key == "PATH" {
			*args = append(*args, "--env", "PATH="+commandPath)
			continue
		}
		*args = append(*args, "--env", key)
	}
}

var baseSandboxEnvKeys = []string{
	"HOME",
	"USER",
	"PATH",
	"TERM",
	"TERMINFO",
	"TERMINFO_DIRS",
	"COLORTERM",
	"LANG",
	"LC_ALL",
	"LC_CTYPE",
	"NIX_REMOTE",
	"SSL_CERT_FILE",
	"NIX_SSL_CERT_FILE",
	"NIX_PATH",
}

func isBaseSandboxEnvKey(key string) bool {
	for _, baseKey := range baseSandboxEnvKeys {
		if key == baseKey {
			return true
		}
	}
	return false
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
