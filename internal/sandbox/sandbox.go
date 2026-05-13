package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
)

func BaseEnv(hostEnv map[string]string) map[string]string {
	reduced := make(map[string]string)
	for _, key := range baseEnvKeys {
		if value, ok := hostEnv[key]; ok {
			reduced[key] = value
		}
	}
	return reduced
}

func LandrunCommand(landrunPath string, hostEnv, commandEnv map[string]string, commandArgs []string) (*exec.Cmd, error) {
	fsArgs, err := filesystemArgs()
	if err != nil {
		return nil, err
	}
	args := landrunArgs(commandEnv, fsArgs, commandArgs)
	cmd := exec.Command(landrunPath, args...)
	cmd.Env = envList(hostEnv)
	return cmd, nil
}

func landrunArgs(commandEnv map[string]string, filesystemArgs []string, commandArgs []string) []string {
	args := make([]string, 0, 64+len(commandArgs))

	keys := make([]string, 0, len(commandEnv))
	for key := range commandEnv {
		if slices.Contains(baseEnvKeys, key) {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		args = append(args, "--env", key+"="+commandEnv[key])
	}

	args = append(args, "--unrestricted-network")
	args = append(args, filesystemArgs...)

	for _, key := range baseEnvKeys {
		if value, exists := commandEnv[key]; exists {
			args = append(args, "--env", key+"="+value)
			continue
		}
		args = append(args, "--env", key)
	}

	args = append(args, "--")
	args = append(args, commandArgs...)
	return args
}

func filesystemArgs() ([]string, error) {
	args := make([]string, 0, 32)
	addExisting := func(flag, path string) {
		if pathExists(path) {
			args = append(args, flag, path)
		}
	}

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
		addExisting("--ro", filepath.Join(xdgConfigHome, "direnv"))
		addExisting("--ro", filepath.Join(xdgConfigHome, "nix"))
		addExisting("--rw", xdgCacheHome)
	}

	for _, path := range []string{
		"/etc/nix",
		"/etc/resolv.conf",
		"/etc/hosts",
		"/etc/nsswitch.conf",
	} {
		addExisting("--ro", path)
	}
	addExisting("--rw", "/nix/var/nix/daemon-socket/socket")

	pwd := os.Getenv("PWD")
	if pwd == "" {
		var err error
		pwd, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to resolve working directory: %w", err)
		}
	}

	args = append(args,
		"--rox", "/nix/store",
		"--rwx", pwd,
		"--rw", "/tmp",
		"--rw", "/proc,/dev,/sys",
	)
	return args, nil
}

var baseEnvKeys = []string{
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

func envList(env map[string]string) []string {
	items := make([]string, 0, len(env))
	for key, value := range env {
		items = append(items, key+"="+value)
	}
	return items
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
