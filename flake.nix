{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs =
    { nixpkgs, ... }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in
    {
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
