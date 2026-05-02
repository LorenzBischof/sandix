# sandix

> **EXPERIMENTAL: This project is an unfinished experiment. It was designed and implemented through an AI-assisted conversation with minimal manual review. It has not been audited, thoroughly tested, or validated for correctness or security. Do not use it in production. Do not rely on this for actual security guarantees.**

sandix sandboxes the binaries that Nix development shells add to your `PATH`.
When you enter a project with `nix develop` or `direnv`, the project's flake,
`.envrc`, and shell hooks can place arbitrary tools in your shell. Running one
of those tools normally gives it the same filesystem access as you, including
your home directory, local keys, API tokens, and private Git repositories.

Unlike full-shell sandboxes such as [sbox](https://github.com/DavHau/sbox), sandix does not put your whole
interactive shell in a box. Your prompt, aliases, shell functions, editor
integration and host commands keep working outside the sandbox; only
binaries introduced by the devshell are routed through sandbox wrappers.

## How it works

When you enter a Nix devshell, tools like `nix develop` or `direnv` add `/nix/store/<hash>/bin` entries to your PATH. Those binaries run with full access to your host filesystem.

sandix intercepts this at two points:

1. **`sandix wrap`** — a stdin filter that preserves the incoming shell script and appends a post-eval PATH rewrite step. The rewrite step only changes `/nix/store/<hash>/...` PATH entries that were added by the script, leaving pre-existing system PATH entries untouched.
2. **`sandix fuse`** — a FUSE daemon that serves the rewritten paths. When a binary is executed through the mount, it transparently serves a wrapper script that runs the real binary through `sandbox-exec`.
3. **`direnv-sandbox`** — a small standalone `DIRENV_BASH` wrapper that runs direnv's `.envrc` evaluation bash process through `sandbox-exec`.

## Trust boundary

```
trusted                               untrusted
──────────────────────────────────────────────────────
~/.zshrc                              .envrc
sandix                                flake.nix
direnv
landrun
```

Flake code and `.envrc` are hostile data sources. sandix leaves their shell output intact, then rewrites the resulting PATH before the hook returns control to the interactive shell.

## Usage

### Start the FUSE daemon

```bash
sandix fuse
```

Mounts at `$XDG_RUNTIME_DIR/sandix` by default. If `XDG_RUNTIME_DIR` is unset,
falls back to `/run/user/<uid>/sandix`. No root required.

### Rewrite a devshell environment

```bash
# with nix develop
nix print-dev-env | sandix wrap | source /dev/stdin
```

For direnv, use one of the modules below. They wrap `programs.direnv.package`
so `direnv export` is piped through `sandix wrap`, and they install a user
`sandix-fuse` systemd service.

### Enter a sandboxed devshell directly

```bash
sandix develop
# or with a specific flake:
sandix develop /path/to/project
```

## Landrun sandbox profile

Each devshell binary runs through `sandbox-exec`, which currently allows:

- `--rox /nix/store` — read-only-executable store access
- `--rwx $PWD` — read-write-executable access to current directory
- `--rw /tmp --rw /proc,/dev,/sys` — read-write access to temporary files and common runtime filesystems
- read-only access to common Nix and network configuration files
- isolated XDG cache, data, and state directories under sandix-specific paths
- unrestricted network access
- a small allowlist of environment variables

## Installation

```nix
# flake.nix
inputs.sandix.url = "github:lorenzbischof/sandix";

sandix.nixosModules.direnv-sandbox
sandix.homeManagerModules.direnv-sandbox
```

```nix
# configuration.nix
{
  imports = [ inputs.sandix.nixosModules.direnv-sandbox ];

  programs.direnv = {
    enable = true;
    sandbox.enable = true;
  };
}
```

```nix
# home.nix
{
  imports = [ inputs.sandix.homeManagerModules.direnv-sandbox ];

  programs.direnv = {
    enable = true;
    sandbox.enable = true;
  };
}
```

## Limitations

**Scripts are not sandboxed.** Scripts that are directly executed from the shell are not sandboxed. See [peninsula](https://github.com/LorenzBischof/peninsula) for a possible workaround.

**Only PATH command lookup is rewritten.** Direct execution of `/nix/store/...` paths, aliases, shell functions, and non-PATH variables are not rewritten by `sandix wrap`.

## Components

| Binary | Description |
|---|---|
| `sandix fuse` | FUSE daemon serving wrapper scripts at the sandboxed store mount |
| `sandix wrap` | stdin filter appending a post-eval PATH rewrite step |
| `sandix rewrite` | rewrites one PATH value relative to a trusted baseline PATH |
| `sandix develop` | drop-in for `nix develop` that applies sandboxing automatically |
| `direnv-sandbox` | standalone `DIRENV_BASH` wrapper that evaluates `.envrc` through `sandbox-exec` |
| `sandbox-exec` | wrapper around `landrun` used by generated command wrappers |
