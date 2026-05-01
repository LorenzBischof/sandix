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
    environment.etc."direnv-sandbox".source = lib.getExe cfg.package;
    environment.variables.DIRENV_BASH = "/etc/direnv-sandbox";
  };
}
