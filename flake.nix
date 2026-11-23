# Nix flake: `nix run github:DipaolaFederico/kmd` (latest commit) or
# `nix run github:DipaolaFederico/kmd/vX.Y.Z` (a release). Builds kmd from
# source with the binary, man page and shell completions. The version is
# taken from the commit (release numbers are only stamped at release time):
# `kmd --version` prints 0-unstable-DATE-COMMIT. See docs/PACKAGING.md.
{
  description = "Fast, beautiful Markdown previewer for the terminal";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
      date = builtins.substring 0 8 (self.lastModifiedDate or "19700101");
      version = "0-unstable-${date}-${self.shortRev or self.dirtyShortRev or "unknown"}";
    in
    {
      packages = forAllSystems (pkgs: {
        default = self.packages.${pkgs.stdenv.hostPlatform.system}.kmd;
        kmd = pkgs.buildGoModule {
          pname = "kmd";
          inherit version;
          src = self;
          vendorHash = null; # no dependencies
          env.CGO_ENABLED = 0;
          ldflags = [
            "-s"
            "-w"
            "-X github.com/DipaolaFederico/kmd/internal/cli.Version=${version}"
          ];
          # tests that bound the time of hostile inputs: allow slow builders
          preCheck = "export KMD_TEST_TIME_SCALE=50";
          nativeBuildInputs = [ pkgs.installShellFiles ];
          postInstall = ''
            installManPage docs/kmd.1
          ''
          + pkgs.lib.optionalString (pkgs.stdenv.buildPlatform.canExecute pkgs.stdenv.hostPlatform) ''
            installShellCompletion --cmd kmd \
              --bash <($out/bin/kmd --completion bash) \
              --zsh <($out/bin/kmd --completion zsh) \
              --fish <($out/bin/kmd --completion fish)
          '';
          meta = {
            description = "Fast, beautiful Markdown previewer for the terminal";
            homepage = "https://github.com/DipaolaFederico/kmd";
            license = pkgs.lib.licenses.mit;
            mainProgram = "kmd";
            platforms = systems;
          };
        };
      });

      checks = forAllSystems (pkgs: {
        kmd = self.packages.${pkgs.stdenv.hostPlatform.system}.kmd;
      });
    };
}
