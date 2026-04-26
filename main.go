package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	gofuse "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	sandixfuse "github.com/lorenzbischof/sandix/internal/fuse"
	"github.com/lorenzbischof/sandix/internal/rewriter"
)

func defaultMountPoint() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return dir + "/sandix-store"
	}
	return os.Getenv("HOME") + "/.local/share/sandix/store"
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: sandix <command> [args]\n")
		fmt.Fprintf(os.Stderr, "Commands: fuse, env, develop\n")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "fuse":
		cmdFuse(os.Args[2:])
	case "env":
		cmdEnv(os.Args[2:])
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

	landrunPath, err := exec.LookPath("landrun")
	if err != nil {
		log.Fatalf("landrun not found in PATH: %v", err)
	}

	root := &sandixfuse.RootNode{LandrunPath: landrunPath}
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

	log.Printf("Serving sandboxed store at %s (landrun: %s)", *mountPoint, landrunPath)
	server.Wait()
}

func cmdEnv(args []string) {
	fs := flag.NewFlagSet("env", flag.ExitOnError)
	mountPoint := fs.String("mount-point", defaultMountPoint(), "sandboxed store mount point")
	fs.Parse(args)

	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Fatalf("Failed to read stdin: %v", err)
	}

	output := rewriter.Rewrite(input, *mountPoint)
	os.Stdout.Write(output)
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

	rewritten := rewriter.Rewrite(output, *mountPoint)

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
