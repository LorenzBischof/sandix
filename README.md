# sandix

> **⚠️ DISCLAIMER: This project was vibe coded. It was designed and implemented through an AI-assisted conversation with minimal manual review. It has not been audited, thoroughly tested, or validated for correctness or security. Use at your own risk. Do not rely on this for actual security guarantees.**

Sandbox Nix devshell binaries using [landrun](https://github.com/Zouuup/landrun).

## How it works

When you enter a Nix devshell, tools like `nix develop` or `direnv` add `/nix/store/<hash>/bin` entries to your PATH. Those binaries run with full access to your host filesystem.

sandix intercepts this at two points:

1. **`sandix env`** — a stdin filter that rewrites `/nix/store/<hash>/bin` entries in PATH to point through a FUSE mount instead.
2. **`sandix fuse`** — a FUSE daemon that serves the rewritten paths. When a binary is executed through the mount, it transparently serves a landrun wrapper script that runs the real binary in a sandbox (read-only-executable `/nix/store`, read-write-executable `$PWD`, read-write `/tmp` and `/dev`).
3. **`direnv-sandbox`** — a small standalone `DIRENV_BASH` wrapper that runs direnv's `.envrc` evaluation bash process through landrun.

## Trust boundary

```
trusted                               untrusted
──────────────────────────────────────────────────────
~/.zshrc                              .envrc
sandix binary                         flake.nix
direnv-sandbox                        shellHook
landrun (runtime dep, from Nix)
```

Flake code and `.envrc` are hostile data sources. Their output is intercepted and processed by sandix before touching the shell.

## Usage

### Start the FUSE daemon

```bash
sandix fuse
```

Mounts at `$XDG_RUNTIME_DIR/sandix-store` by default. No root required.

### Rewrite a devshell environment

```bash
# with nix develop
nix print-dev-env | sandix env | source /dev/stdin

# optional: also rewrite PATH entries from direnv through sandix
_direnv_hook_sandboxed() {
    local output
    output=$(direnv export zsh 2>/dev/null)
    if [[ -n "$output" ]]; then
        output=$(echo "$output" | sandix env)
    fi
    eval "$output"
}
precmd_functions+=(_direnv_hook_sandboxed)
```

### Enter a sandboxed devshell directly

```bash
sandix develop
# or with a specific flake:
sandix develop /path/to/project
```

## Landrun sandbox profile

Each devshell binary runs with:

- `--rox /nix/store` — read-only-executable store access
- `--rwx $PWD` — read-write-executable access to current directory
- `--rw /tmp --rw /dev` — read-write access to temporary files and device nodes
- `--env <name>` for each inherited environment variable

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

## Components

| Binary | Description |
|---|---|
| `sandix fuse` | FUSE daemon serving wrapper scripts at the sandboxed store mount |
| `sandix env` | stdin filter rewriting PATH entries to route through the FUSE mount |
| `sandix develop` | drop-in for `nix develop` that applies sandboxing automatically |
| `direnv-sandbox` | standalone `DIRENV_BASH` wrapper that evaluates `.envrc` through landrun |
