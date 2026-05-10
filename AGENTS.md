# Repository Instructions

## Security Posture

sandix is security-oriented tooling. Treat `.envrc`, flake code, shell hooks,
and project-controlled environment variables as hostile inputs.

Prefer security by design:

- Fail closed instead of silently ignoring unexpected input.
- Strip, reject, or error on untrusted paths rather than preserving them.
- Keep trust boundaries explicit in code, tests, and documentation.
- Avoid adding bypasses for convenience unless the bypass is explicit,
  documented, tested, and opt-in.
- When behavior is uncertain, choose the option that reduces host exposure.

For `PATH` handling specifically, do not preserve non-store paths from
direnv-managed environments. A path outside `/nix/store` can hide unsandboxed
executables or symlink to store executables while bypassing wrapper generation.
