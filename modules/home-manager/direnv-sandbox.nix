{ self }:
{
  lib,
  config,
  pkgs,
  ...
}:
let
  cfg = config.programs.direnv.sandbox;
in
let
  stablePath = "${config.home.homeDirectory}/.local/share/direnv-sandbox/bash";
in
{
  options.programs.direnv.sandbox = {
    enable = lib.mkEnableOption ''
      sandboxing direnv's bash subprocess via DIRENV_BASH
    '';

    package = lib.mkOption {
      type = lib.types.package;
      default = self.packages.${pkgs.system}.direnv-sandbox;
      defaultText = lib.literalExpression ''inputs.sandix.packages.${pkgs.system}.direnv-sandbox'';
      description = ''
        The `direnv-sandbox` wrapper package used for `DIRENV_BASH`.
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    home.file.".local/share/direnv-sandbox/bash".source = lib.getExe cfg.package;
    home.sessionVariables.DIRENV_BASH = stablePath;
  };
}
