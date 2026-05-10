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
  trustedPackageNames = lib.concatStringsSep "," cfg.wrap.trustedPackageNames;
  supportedHookShells = [
    "bash"
    "elvish"
    "fish"
    "murex"
    "tcsh"
    "zsh"
  ];
  patchedHooks = pkgs.runCommand "sandix-direnv-hooks" { } ''
    set -eu

    mkdir -p "$out"
    patch_hook() {
      shell="$1"
      hook="$out/$shell"
      ${lib.getExe cfg.wrap.direnvPackage} hook "$shell" \
        | ${pkgs.gnused}/bin/sed -E \
          -e "s#\"${lib.getExe cfg.wrap.direnvPackage}\" export ([^[:space:]\)]+)#${lib.getExe cfg.wrap.package} direnv-export --trusted-package-names ${lib.escapeShellArg trustedPackageNames} \1#g" \
          -e "s#${lib.getExe cfg.wrap.direnvPackage} export ([^[:space:]\`]+)#${lib.getExe cfg.wrap.package} direnv-export --trusted-package-names ${lib.escapeShellArg trustedPackageNames} \1#g" \
        > "$hook"

      if ! ${pkgs.gnugrep}/bin/grep -Fq "${lib.getExe cfg.wrap.package} direnv-export" "$hook"; then
        echo "failed to patch direnv hook for $shell" >&2
        exit 1
      fi
      if ${pkgs.gnugrep}/bin/grep -Fq '"${lib.getExe cfg.wrap.direnvPackage}" export' "$hook" \
        || ${pkgs.gnugrep}/bin/grep -Fq "${lib.getExe cfg.wrap.direnvPackage} export" "$hook"; then
        echo "direnv hook for $shell still contains an unpatched direnv export call" >&2
        exit 1
      fi
    }

    ${lib.concatMapStringsSep "\n" (shell: "patch_hook ${lib.escapeShellArg shell}") supportedHookShells}
  '';
  patchedDirenv = pkgs.writeShellScriptBin "direnv" ''
      if [[ "''${1-}" == "hook" ]]; then
        case "''${2-}" in
          ${lib.concatMapStringsSep "\n          " (shell: "${shell}) cat ${patchedHooks}/${shell} ;;") supportedHookShells}
          *) echo "unsupported direnv hook shell: ''${2-}" >&2; exit 1 ;;
        esac
      else
        exec ${lib.getExe cfg.wrap.direnvPackage} "$@"
      fi
  '';
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
          Whether to wrap `programs.direnv.package` with one that runs
          `direnv export` through `sandix direnv-export`.
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

      trustedPackageNames = lib.mkOption {
        type = lib.types.listOf lib.types.str;
        default = [
          "coreutils"
          "gnugrep"
          "gnused"
          "findutils"
        ];
        defaultText = lib.literalExpression ''[ "coreutils" "gnugrep" "gnused" "findutils" ]'';
        description = ''
          Package names that may remain unwrapped in the interactive PATH when
          their concrete devshell store paths verify against the official
          cache.nixos.org signing key.

          These names are matched against the package name parsed from each
          devshell PATH entry's store path. If signature verification fails, the
          PATH entry is wrapped like any other devshell package.
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
  };
}
