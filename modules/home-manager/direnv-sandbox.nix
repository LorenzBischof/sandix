{ self }:
{
  lib,
  config,
  pkgs,
  ...
}:
let
  cfg = config.programs.direnv.sandbox;
  direnvCfg = config.programs.direnv;
  stablePath = "${config.home.homeDirectory}/.local/share/direnv-sandbox/bash";
  trustedPath = lib.makeBinPath cfg.wrap.trustedPackages;
  patchedDirenv = pkgs.writeShellApplication {
    name = "direnv";
    text = ''
      if [[ "''${1-}" == "hook" ]]; then
        ${lib.getExe cfg.wrap.direnvPackage} hook "$2" \
          | ${pkgs.gnused}/bin/sed -E "s#\"${lib.getExe cfg.wrap.direnvPackage}\" export ([^[:space:]\)]+)#\"${lib.getExe cfg.wrap.direnvPackage}\" export \1 | ${lib.getExe cfg.wrap.package} wrap --trusted-path ${lib.escapeShellArg trustedPath}#g"
      else
        exec ${lib.getExe cfg.wrap.direnvPackage} "$@"
      fi
    '';
  };
in
{
  options.programs.direnv.sandbox = {
    enable = lib.mkEnableOption ''
      sandboxing direnv's bash subprocess via DIRENV_BASH
    '';

    package = lib.mkOption {
      type = lib.types.package;
      default = self.packages.${pkgs.system}.direnv-sandbox;
      defaultText = lib.literalExpression "inputs.sandix.packages.${pkgs.system}.direnv-sandbox";
      description = ''
        The `direnv-sandbox` wrapper package used for `DIRENV_BASH`.
      '';
    };

    wrap = {
      enable = lib.mkOption {
        type = lib.types.bool;
        default = true;
        description = ''
          Whether to wrap `programs.direnv.package` with one that pipes
          `direnv export` through `sandix wrap`.
        '';
      };

      package = lib.mkOption {
        type = lib.types.package;
        default = self.packages.${pkgs.system}.default;
        defaultText = lib.literalExpression "inputs.sandix.packages.${pkgs.system}.default";
        description = ''
          The `sandix` package providing the `wrap` command.
        '';
      };

      direnvPackage = lib.mkOption {
        type = lib.types.package;
        default = pkgs.direnv;
        defaultText = lib.literalExpression "pkgs.direnv";
        description = ''
          The real `direnv` package to wrap.
        '';
      };

      trustedPackages = lib.mkOption {
        type = lib.types.listOf lib.types.package;
        default = [
          pkgs.coreutils
          pkgs.gnugrep
          pkgs.gnused
          pkgs.findutils
        ];
        defaultText = lib.literalExpression "[ pkgs.coreutils pkgs.gnugrep pkgs.gnused pkgs.findutils ]";
        description = ''
          Trusted host shell tools to prepend to the interactive PATH only.
          These tools are not added to sandboxed child
          processes.
        '';
      };

    };
  };

  config = lib.mkIf cfg.enable {
    home.file.".local/share/direnv-sandbox/bash".source = lib.getExe cfg.package;
    home.sessionVariables.DIRENV_BASH = stablePath;

    programs.direnv.package = lib.mkIf (cfg.wrap.enable && direnvCfg.enable) (
      lib.mkForce patchedDirenv
    );

    systemd.user.services.sandix-fuse = lib.mkIf cfg.wrap.enable {
      Unit = {
        Description = "sandix FUSE daemon";
        Documentation = "https://github.com/lorenzbischof/sandix";
      };

      Service = {
        Environment = "PATH=/run/wrappers/bin";
        ExecStart = "${lib.getExe cfg.wrap.package} fuse";
        Restart = "on-failure";
      };

      Install.WantedBy = [ "default.target" ];
    };
  };
}
