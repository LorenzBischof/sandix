{
  pkgs ? import <nixpkgs> { },
  sandix,
  direnv-sandbox,
  direnv-sandbox-module,
}:

pkgs.testers.runNixOSTest {
  name = "direnv-sandbox";

  nodes.machine =
    { pkgs, ... }:
    {
      imports = [ direnv-sandbox-module ];

      environment.systemPackages = [
        direnv-sandbox
      ];

      programs.direnv = {
        enable = true;
        sandbox.enable = true;
      };

      users.users.testuser = {
        isNormalUser = true;
        home = "/home/testuser";
      };

      system.activationScripts.sshDir = ''
        mkdir -p /home/testuser/.ssh
        echo "SECRET_KEY" > /home/testuser/.ssh/id_ed25519
        chown -R testuser:users /home/testuser/.ssh
        chmod 700 /home/testuser/.ssh
        chmod 600 /home/testuser/.ssh/id_ed25519
      '';
    };

  testScript = ''
    machine.wait_for_unit("multi-user.target")

    # Sanity: the secret is readable without sandboxing.
    result = machine.succeed(
        "su - testuser -c 'cat ~/.ssh/id_ed25519'"
    ).strip()
    assert result == "SECRET_KEY", f"Expected plaintext secret, got: {result}"

    # Direct wrapper check: the sandboxed bash must not be able to read ~/.ssh.
    machine.fail(
        "su - testuser -c 'cd /tmp && direnv-sandbox -c \"cat ~/.ssh/id_ed25519 >/dev/null\"'"
    )

    # Direnv integration check: sandboxed .envrc evaluation must not be able to read ~/.ssh.
    machine.succeed(
        "su - testuser -c 'mkdir -p ~/.config/direnv && "
        "printf \"[whitelist]\\nprefix = [\\\"/home/testuser\\\"]\\n\" > ~/.config/direnv/direnv.toml'"
    )
    machine.succeed("su - testuser -c 'mkdir -p ~/test-project'")
    machine.succeed(
        "su - testuser -c 'cat > ~/test-project/.envrc << \"EOF\"\n"
        "export SSH_KEY_CONTENT=\"$(cat ~/.ssh/id_ed25519 2>/dev/null || true)\"\n"
        "export PROJECT_ONLY=\"from-envrc\"\n"
        "export PATH=\"${pkgs.writeShellScriptBin "project-env-printer" ''
          printf "%s|%s" "$PROJECT_ONLY" "$HOST_ONLY"
        ''}/bin:$PATH\"\n"
        "EOF'"
    )
    machine.succeed("su - testuser -c 'cd ~/test-project && direnv allow .'")
    machine.succeed("su - testuser -c 'mkdir -p ~/test-project/nested-project'")
    machine.succeed(
        "su - testuser -c 'cat > ~/test-project/nested-project/.envrc << \"EOF\"\n"
        "export PROJECT_ONLY=\"from-nested-envrc\"\n"
        "export PATH=\"${pkgs.writeShellScriptBin "nested-env-printer" ''
          printf "%s|%s" "$PROJECT_ONLY" "$HOST_ONLY"
        ''}/bin:$PATH\"\n"
        "EOF'"
    )
    machine.succeed("su - testuser -c 'cd ~/test-project/nested-project && direnv allow .'")

    result = machine.succeed(
        "su - testuser -c 'cd ~/test-project && unset DIRENV_BASH && eval \"$(direnv export bash)\" && echo SSH_KEY_CONTENT=$SSH_KEY_CONTENT'"
    ).strip()
    assert "SSH_KEY_CONTENT=SECRET_KEY" in result, f"Expected unsandboxed direnv to read ~/.ssh, got: {result}"

    result = machine.succeed(
        "su - testuser -c 'printf \"%s\" \"$DIRENV_BASH\"'"
    ).strip()
    assert result == "/etc/direnv-sandbox", (
        f"Expected DIRENV_BASH to point to the stable managed symlink, got: {result}"
    )

    result = machine.succeed(
        "su - testuser -c 'cd ~/test-project && eval \"$(direnv export bash)\" && echo SSH_KEY_CONTENT=$SSH_KEY_CONTENT'"
    ).strip()
    assert "SSH_KEY_CONTENT=" in result, f"Expected sandboxed direnv to block ~/.ssh, got: {result}"

    result = machine.succeed(
        "su - testuser -c 'cd ~/test-project && export HOST_ONLY=from-host && "
        "eval \"$(direnv export bash)\" && printf \"%s|%s\" \"$HOST_ONLY\" \"$PROJECT_ONLY\"'"
    ).strip()
    assert result == "from-host|from-envrc", (
        f"Expected interactive shell to keep host vars and add project vars, got: {result}"
    )

    sandbox_exec = "${pkgs.lib.getExe sandix} exec ${pkgs.bash}/bin/bash -c"
    wrapped_export = "${pkgs.lib.getExe sandix} direnv-export bash"

    minimal_path = "/run/current-system/sw/bin"
    result = machine.succeed(
        "su - testuser -c 'cd /tmp && env -i HOME=/home/testuser USER=testuser PATH="
        + minimal_path
        + " ${pkgs.lib.getExe sandix} exec ${pkgs.coreutils}/bin/env'"
    ).strip()
    env_items = dict(line.split("=", 1) for line in result.splitlines() if "=" in line)
    expected_env = {
        "HOME": "/home/testuser",
        "PATH": minimal_path,
        "USER": "testuser",
    }
    assert env_items == expected_env, (
        f"Expected packaged sandix wrapper not to add environment variables or PATH entries, got: {env_items}"
    )

    result = machine.succeed(
        "su - testuser -c 'cd ~/test-project && export HOST_ONLY=from-host && "
        "eval \"$(direnv export bash)\" && "
        + sandbox_exec
        + " \"printf \\\"%s|%s\\\" \\\"\\$PROJECT_ONLY\\\" \\\"\\$HOST_ONLY\\\"\"'"
    ).strip()
    assert result == "from-envrc|", (
        f"Expected sandboxed commands to receive project overlay but not host-only vars, got: {result}"
    )

    result = machine.succeed(
        "su - testuser -c 'export HOST_ONLY=from-host && "
        "cd ~/test-project && eval \"$(direnv export bash)\" && "
        "cd ~/test-project/nested-project && eval \"$(direnv export bash)\" && "
        + sandbox_exec
        + " \"printf \\\"%s|%s\\\" \\\"\\$PROJECT_ONLY\\\" \\\"\\$HOST_ONLY\\\"\"'"
    ).strip()
    assert result == "from-nested-envrc|", (
        f"Expected nested direnv to use the current DIRENV_DIFF without leaking host vars, got: {result}"
    )

    result = machine.succeed(
        "su - testuser -c 'cd ~/test-project && eval \"$("
        + wrapped_export
        + ")\" && "
        "cd ~/test-project/nested-project && eval \"$("
        + wrapped_export
        + ")\" && "
        "cd .. && eval \"$("
        + wrapped_export
        + ")\" && "
        "project-env-printer >/dev/null || printf missing-parent && "
        "if command -v nested-env-printer >/dev/null; then printf leak; else printf ok; fi'"
    ).strip()
    assert result == "ok", (
        f"Expected unloading nested direnv to remove the nested PATH entry while keeping the parent PATH, got: {result}"
    )
  '';
}
