{
  description = "Zen - simple, free and efficient ad-blocker and privacy guard";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      # zen-adblocker, not zen: matching the desktop entry and icon name, and
      # distinctive enough for nixpkgs, where a bare "zen" would be ambiguous.
      packages = forAllSystems (pkgs: rec {
        zen-adblocker = pkgs.callPackage ./nix/package.nix { };
        default = zen-adblocker;
      });

      overlays.default = final: prev: {
        zen-adblocker = final.callPackage ./nix/package.nix { };
      };

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = with pkgs; [
            go_1_26
            nodejs_24
            go-task
            pkg-config
            gtk3
            webkitgtk_4_1
          ];
        };
      });
    };
}
