# sandix

> **EXPERIMENTAL: This project is an unfinished experiment. It was designed and implemented through an AI-assisted conversation with minimal manual review. It has not been audited, thoroughly tested, or validated for correctness or security. Do not use it in production. Do not rely on this for actual security guarantees.**

sandix sandboxes the binaries that direnv-backed Nix development shells add to
your `PATH`. When you enter a project with `direnv`, the project's flake,
`.envrc`, and shell hooks can place arbitrary tools in your shell. Running one
of those tools normally gives it the same filesystem access as you, including
your home directory, local keys, API tokens, and private Git repositories.

Unlike full-shell sandboxes such as [sbox](https://github.com/DavHau/sbox), sandix does not put your whole
interactive shell in a box. Your prompt, aliases, shell functions, editor
integration and host commands keep working outside the sandbox; only
binaries introduced by the devshell are routed through sandbox wrappers.

## How it works

When you enter a Nix devshell through `direnv`, it adds `/nix/store/<hash>/bin` entries to your PATH. Those binaries run with full access to your host filesystem.

sandix intercepts this at two points:

1. **`sandix wrap`** — a stdin filter that preserves the incoming shell script and appends a post-eval PATH rewrite step. The rewrite step only changes `/nix/store/<hash>/...` PATH entries that were added by the script, leaving pre-existing system PATH entries untouched.
2. **`sandix fuse`** — a FUSE daemon that serves the rewritten paths. When a binary is executed through the mount, it transparently serves a wrapper script that runs the real binary through `sandix exec`.
3. **`direnv-sandbox`** — a small standalone `DIRENV_BASH` wrapper that runs direnv's `.envrc` evaluation bash process through `sandix exec`.

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

Mounts at `$XDG_RUNTIME_DIR/sandix/store` by default. If `XDG_RUNTIME_DIR` is
unset, falls back to `/run/user/<uid>/sandix/store`. No root required.

### Sandbox direnv

Use one of the modules below. They wrap `programs.direnv.package` so
`direnv export` is piped through `sandix wrap`, configure `DIRENV_BASH` to run
`.envrc` evaluation through `sandix exec`, and install a `sandix-fuse`
systemd service.

## Landrun sandbox profile

Each devshell binary runs through `sandix exec`, which currently allows:

- `--rox /nix/store` — read-only-executable store access
- `--rwx $PWD` — read-write-executable access to current directory
- `--rw /tmp --rw /proc,/dev,/sys` — read-write access to temporary files and common runtime filesystems
- read-only access to common Nix and network configuration files
- read-write access to `$XDG_CACHE_HOME` or `$HOME/.cache` for Nix fetcher cache compatibility
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

**Sandboxed direnv evaluation has a reduced environment.** `direnv-sandbox` returns the full host environment with the project environment layered on top. Sandboxed binaries reuse direnv's `DIRENV_DIFF` instead of a sandix overlay file. See [Sandboxed direnv Environment Design](docs/sandboxed-direnv-environment.md). Variables outside sandix's reduced evaluator environment are still unavailable while `.envrc` itself is evaluated.

**XDG paths are not redirected.** Landlock restricts access to paths but does not remap paths, so sandix does not rewrite `XDG_CACHE_HOME`, `XDG_DATA_HOME`, or `XDG_STATE_HOME` to sandbox-specific directories. For Nix compatibility, sandix grants read-write access to the existing XDG cache directory. XDG data and state directories remain unavailable unless they are otherwise allowed by the sandbox profile.

## Components

| Binary | Description |
|---|---|
| `sandix fuse` | FUSE daemon serving wrapper scripts at the sandboxed store mount |
| `sandix wrap` | stdin filter appending a post-eval PATH rewrite step |
| `sandix rewrite` | rewrites one PATH value through the sandboxed store mount |
| `sandix direnv-path` | rewrites the current direnv PATH from `DIRENV_DIFF` |
| `sandix exec` | wrapper around `landrun` used by generated command wrappers |
| `direnv-sandbox` | standalone `DIRENV_BASH` wrapper that evaluates `.envrc` through `sandix exec` |
