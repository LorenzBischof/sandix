package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	gofuse "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	sandixfuse "github.com/lorenzbischof/sandix/internal/fuse"
	"github.com/lorenzbischof/sandix/internal/rewriter"
)

func defaultMountPoint() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "sandix")
	}
	return filepath.Join("/run/user", strconv.Itoa(os.Getuid()), "sandix")
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: sandix <command> [args]\n")
		fmt.Fprintf(os.Stderr, "Commands: fuse, wrap, rewrite, develop\n")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "fuse":
		cmdFuse(os.Args[2:])
	case "wrap":
		cmdWrap(os.Args[2:])
	case "rewrite":
		cmdRewrite(os.Args[2:])
	case "develop":
		cmdDevelop(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func cmdFuse(args []string) {
	fs := flag.NewFlagSet("fuse", flag.ExitOnError)
	mountPoint := fs.String("mount-point", defaultMountPoint(), "FUSE mount point")
	debug := fs.Bool("debug", false, "enable FUSE debug logging")
	fs.Parse(args)

	if err := os.MkdirAll(*mountPoint, 0755); err != nil {
		log.Fatalf("Failed to create mount point %s: %v", *mountPoint, err)
	}

	sandboxExecPath, err := exec.LookPath("sandbox-exec")
	if err != nil {
		log.Fatalf("sandbox-exec not found in PATH: %v", err)
	}

	root := &sandixfuse.RootNode{SandboxExecPath: sandboxExecPath}
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

	log.Printf("Serving sandboxed store at %s (sandbox-exec: %s)", *mountPoint, sandboxExecPath)
	server.Wait()
}

func cmdWrap(args []string) {
	fs := flag.NewFlagSet("wrap", flag.ExitOnError)
	mountPoint := fs.String("mount-point", defaultMountPoint(), "sandboxed store mount point")
	baseline := fs.String("baseline", os.Getenv("PATH"), "trusted baseline PATH")
	trustedPath := fs.String("trusted-path", "", "trusted host PATH entries to prepend outside the sandbox")
	fs.Parse(args)

	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Fatalf("Failed to read stdin: %v", err)
	}

	sandixPath, err := os.Executable()
	if err != nil {
		log.Fatalf("Failed to resolve sandix executable: %v", err)
	}

	output := rewriter.AppendPathRewrite(input, *baseline, *mountPoint, sandixPath, *trustedPath)
	os.Stdout.Write(output)
}

func cmdRewrite(args []string) {
	fs := flag.NewFlagSet("rewrite", flag.ExitOnError)
	mountPoint := fs.String("mount-point", defaultMountPoint(), "sandboxed store mount point")
	baseline := fs.String("baseline", os.Getenv("PATH"), "trusted baseline PATH")
	fs.Parse(args)

	pathValue := os.Getenv("PATH")
	switch fs.NArg() {
	case 0:
	case 1:
		pathValue = fs.Arg(0)
	default:
		log.Fatalf("Usage: sandix rewrite [--mount-point PATH] [--baseline PATH] [PATH]")
	}

	fmt.Print(rewriter.RewritePath(pathValue, *baseline, *mountPoint))
}

func cmdDevelop(args []string) {
	fs := flag.NewFlagSet("develop", flag.ExitOnError)
	mountPoint := fs.String("mount-point", defaultMountPoint(), "sandboxed store mount point")
	fs.Parse(args)

	// Run nix print-dev-env with remaining args.
	nixArgs := append([]string{"print-dev-env"}, fs.Args()...)
	cmd := exec.Command("nix", nixArgs...)
	cmd.Stderr = os.Stderr
	output, err := cmd.Output()
	if err != nil {
		log.Fatalf("nix print-dev-env failed: %v", err)
	}

	sandixPath, err := os.Executable()
	if err != nil {
		log.Fatalf("Failed to resolve sandix executable: %v", err)
	}
	rewritten := rewriter.AppendPathRewrite(output, os.Getenv("PATH"), *mountPoint, sandixPath, "")

	// Eval the rewritten environment and exec into a new shell.
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	shellCmd := exec.Command(shell, "-c", string(rewritten)+"\nexec "+shell)
	shellCmd.Stdin = os.Stdin
	shellCmd.Stdout = os.Stdout
	shellCmd.Stderr = os.Stderr
	if err := shellCmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		log.Fatalf("Shell failed: %v", err)
	}
}
