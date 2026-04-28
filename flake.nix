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
        in
        {
          default = pkgs.buildGoModule {
            pname = "sandix";
            version = "0.1.0";
            src = ./.;
            vendorHash = "sha256-9e1HDWC5Cw1gbpM0Vs/7k+maxlNvi7fRR+HNFa8DtXs=";
            env.CGO_ENABLED = 0;
            nativeBuildInputs = [ pkgs.makeWrapper ];
            postInstall = ''
              wrapProgram $out/bin/sandix \
                --prefix PATH : ${pkgs.lib.makeBinPath [ pkgs.landrun ]}
            '';
          };

          direnv-sandbox = pkgs.writeShellApplication {
            name = "direnv-sandbox";
            runtimeInputs = [
              pkgs.bash
              pkgs.landrun
            ];
            text = builtins.readFile ./scripts/direnv-sandbox;
          };
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
