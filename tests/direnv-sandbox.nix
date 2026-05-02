{
  pkgs ? import <nixpkgs> { },
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
        "EOF'"
    )
    machine.succeed("su - testuser -c 'cd ~/test-project && direnv allow .'")

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

    machine.succeed("test -e /etc/systemd/user/sandix-fuse.service")
    machine.succeed("grep -F 'ExecStart=' /etc/systemd/user/sandix-fuse.service | grep -F 'sandix fuse'")
    machine.succeed("grep -F 'Environment=' /etc/systemd/user/sandix-fuse.service | grep -F '/run/wrappers/bin'")

    result = machine.succeed(
        "su - testuser -c 'cd ~/test-project && eval \"$(direnv export bash)\" && echo SSH_KEY_CONTENT=$SSH_KEY_CONTENT'"
    ).strip()
    assert "SSH_KEY_CONTENT=" in result, f"Expected sandboxed direnv to block ~/.ssh, got: {result}"
  '';
}
