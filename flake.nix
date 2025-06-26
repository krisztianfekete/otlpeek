{
  description = "A Nix-flake-based Go development environment for otlpeek";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = {
    self,
    nixpkgs,
    ...
  }: let
    supportedSystems = ["x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin"];
    ocbHash = "e6f23dc08d3624daab7094b701aa3954923c6bbb"; #0.126.0, see https://lazamar.co.uk/nix-versions/
    ocbTarball = fetchTarball {
      url = "https://github.com/NixOS/nixpkgs/archive/${ocbHash}.tar.gz";
      sha256 = "0m0xmk8sjb5gv2pq7s8w7qxf7qggqsd3rxzv3xrqkhfimy2x7bnx";
    };

    forEachSystem = systems: f:
      nixpkgs.lib.genAttrs systems (system:
        let
          pkgs = import nixpkgs { inherit system; };
          ocbPkgs = import ocbTarball { inherit system; };
        in
        f {
          inherit system pkgs ocbPkgs;
        });

  in {
    devShells = forEachSystem supportedSystems ({pkgs, ocbPkgs, ...}: {
      default = pkgs.mkShell {

        # Make Go debugging work: https://github.com/go-delve/delve/issues/3085
        hardeningDisable = [ "fortify" ];
        packages = with pkgs; [
          go_1_23
          gopls
          gomodifytags
        ] ++ (with ocbPkgs; [
          opentelemetry-collector-builder
        ]);
      };
    });
  };
}
