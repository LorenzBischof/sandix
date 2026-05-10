{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs =
    { nixpkgs, self, ... }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in
    {
      nixosModules = {
        direnv-sandbox = import ./modules/nixos/direnv-sandbox.nix { inherit self; };
        default = self.nixosModules.direnv-sandbox;
      };

      homeManagerModules = {
        direnv-sandbox = import ./modules/home-manager/direnv-sandbox.nix { inherit self; };
        default = self.homeManagerModules.direnv-sandbox;
      };

      packages = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          wrapperBuilder = pkgs.writeShellScriptBin "sandix-wrapper-builder" ''
            set -eu
            export PATH=${pkgs.lib.makeBinPath [ pkgs.coreutils ]}
            exec ${pkgs.runtimeShell} -eu -c "$BUILD_SCRIPT"
          '';
        in
        rec {
          sandix-unwrapped = pkgs.buildGoModule {
            pname = "sandix";
            version = "0.1.0";
            src = ./.;
            vendorHash = null;
            env.CGO_ENABLED = 0;
            meta.mainProgram = "sandix";
          };

          default = pkgs.writeShellScriptBin "sandix" ''
            set -eu

            self="$(${pkgs.coreutils}/bin/readlink -f "$0")"
            command="''${1-}"
            if [ -z "$command" ]; then
              exec ${pkgs.lib.getExe sandix-unwrapped}
            fi
            shift

            exec ${pkgs.lib.getExe sandix-unwrapped} "$command" \
              --bash ${pkgs.bash}/bin/bash \
              --nix ${pkgs.lib.getExe pkgs.nix} \
              --builder ${pkgs.lib.getExe wrapperBuilder} \
              --direnv ${pkgs.lib.getExe pkgs.direnv} \
              --landrun ${pkgs.lib.getExe pkgs.landrun} \
              --sandix "$self" \
              --shell ${pkgs.runtimeShell} \
              "$@"
          '';

          direnv-sandbox = pkgs.writeShellScriptBin "direnv-sandbox" ''
              exec ${pkgs.lib.getExe default} direnv-bash -- "$@"
          '';
        }
      );

      checks = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          direnv-sandbox = import ./tests/direnv-sandbox.nix {
            inherit pkgs;
            sandix = self.packages.${system}.default;
            direnv-sandbox = self.packages.${system}.direnv-sandbox;
            direnv-sandbox-module = self.nixosModules.direnv-sandbox;
          };
        }
      );

      devShells = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = pkgs.mkShell {
            packages = [
              pkgs.go
              pkgs.landrun
            ];
          };
        }
      );
    };
}
