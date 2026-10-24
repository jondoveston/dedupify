{
  description = "dedupify: find duplicate files by content hash";

  inputs = {
    # Stable channel: its default `go` is what we build and release with.
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";
  };

  outputs =
    {
      self,
      nixpkgs,
    }:
    let
      # The machines that can *run* `nix build` on this flake -- not the
      # release targets (see `packages` below), which Go cross-compiles
      # regardless of which of these is building.
      buildSystems = [
        "x86_64-linux"
        "aarch64-linux"
        "aarch64-darwin"
      ];

      version = nixpkgs.lib.fileContents ./VERSION;

      # self.shortRev only exists on a clean tree; a dirty tree gets
      # self.dirtyShortRev (already suffixed "-dirty" by Nix) instead, and
      # a from-scratch repo with zero commits has neither.
      commit = self.shortRev or self.dirtyShortRev or "unknown";
      date =
        let
          t = builtins.substring 0 14 (self.lastModifiedDate or "");
          y = builtins.substring 0 4 t;
          m = builtins.substring 4 2 t;
          d = builtins.substring 6 2 t;
          hh = builtins.substring 8 2 t;
          mm = builtins.substring 10 2 t;
          ss = builtins.substring 12 2 t;
        in
        if t == "" then "unknown" else "${y}-${m}-${d}T${hh}:${mm}:${ss}Z";

      repo = "https://github.com/jondoveston/dedupify";

      mkDedupify =
        pkgs:
        pkgs.buildGoModule (finalAttrs: {
          pname = "dedupify";
          inherit version;

          # Only the files that affect the build, so editing the README,
          # workflows or this flake does not trigger a rebuild.
          src = pkgs.lib.fileset.toSource {
            root = ./.;
            fileset = pkgs.lib.fileset.unions [
              ./go.mod
              ./go.sum
              (pkgs.lib.fileset.fileFilter (file: file.hasExt "go") ./.)
            ];
          };

          # `nix build` fails on the first run with a "hash mismatch" error
          # reporting the correct value under "got:" -- `task nix:vendor-hash`
          # does that and updates this line. Must be refreshed whenever
          # go.mod/go.sum change.
          vendorHash = "sha256-sw43LXWiV3s79aexUMH7+bKh3A3ujI7jAHyC8sSK9dE=";

          subPackages = [ "." ];

          env.CGO_ENABLED = 0;

          ldflags = [
            "-s"
            "-w"
            "-X main.version=${version}"
            "-X main.commit=${commit}"
            "-X main.date=${date}"
          ];

          meta = {
            description = "Find duplicate files by content hash";
            homepage = repo;
            mainProgram = "dedupify";
          };
        });

      # Release binaries, cross-compiled by Go itself rather than by
      # nixpkgs's pkgsCross. With CGO off, Go needs no C toolchain, so setting
      # GOOS/GOARCH on the *native* package set is enough. goos/goarch are
      # Go's names (linux/darwin, arm64/amd64), not Nix's.
      #
      # This must be overrideAttrs, not env.GOOS passed to mkDedupify:
      # buildGoModule does `args.env // { inherit (go) GOOS GOARCH; }`, so the
      # host's values always win over anything passed in. overrideAttrs runs
      # after that merge, and leaves the vendored-modules derivation alone, so
      # one vendorHash covers every target.
      mkDedupifyFor =
        pkgs:
        { goos, goarch }:
        (mkDedupify pkgs).overrideAttrs (old: {
          env = old.env // {
            GOOS = goos;
            GOARCH = goarch;
          };
          # The tests are native binaries and cannot run for another target.
          doCheck = false;
          # `go install` puts cross-compiled binaries in bin/<GOOS>_<GOARCH>/.
          # buildGoModule only flattens that when stdenv itself is cross,
          # which is not the case here. When the target happens to equal the
          # builder the directory simply doesn't exist, hence the test.
          postBuild = (old.postBuild or "") + ''
            dir="$GOPATH/bin/${goos}_${goarch}"
            if [ -d "$dir" ]; then
              mv "$dir"/* "$GOPATH/bin/"
              rmdir "$dir"
            fi
          '';
        });

      # A reproducible tarball: sorted entries, fixed owner and mtime, and no
      # name or timestamp in the gzip header, so identical inputs always give
      # identical bytes (and so an identical sha256 in the Homebrew formula).
      mkTarball =
        {
          pkgs,
          dedupify,
          goos,
          arch,
        }:
        pkgs.runCommand "dedupify-${version}-${goos}-${arch}.tar.gz"
          {
            nativeBuildInputs = [
              pkgs.buildPackages.gnutar
              pkgs.buildPackages.gzip
            ];
          }
          ''
            dir="dedupify-${version}-${goos}-${arch}"
            mkdir -p "$dir"
            cp ${dedupify}/bin/dedupify "$dir/"
            cp ${./README.md} "$dir/README.md"
            ${pkgs.lib.optionalString (builtins.pathExists ./LICENSE) ''cp ${./LICENSE} "$dir/LICENSE"''}
            tar \
              --sort=name --owner=0 --group=0 --numeric-owner \
              --mtime="@${toString (self.lastModified or 0)}" \
              -cf - "$dir" | gzip -n > "$out"
          '';

      forAllSystems = nixpkgs.lib.genAttrs buildSystems;
    in
    {
      packages = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          native = mkDedupify pkgs;

          dedupify-aarch64-linux = mkDedupifyFor pkgs {
            goos = "linux";
            goarch = "arm64";
          };
          dedupify-x86_64-linux = mkDedupifyFor pkgs {
            goos = "linux";
            goarch = "amd64";
          };
          # No x86_64-darwin: Go still supports it, but nixpkgs is dropping
          # Intel Macs, so it isn't worth shipping.
          dedupify-aarch64-darwin = mkDedupifyFor pkgs {
            goos = "darwin";
            goarch = "arm64";
          };

          tarball-aarch64-linux = mkTarball {
            inherit pkgs;
            dedupify = dedupify-aarch64-linux;
            goos = "linux";
            arch = "aarch64";
          };
          tarball-x86_64-linux = mkTarball {
            inherit pkgs;
            dedupify = dedupify-x86_64-linux;
            goos = "linux";
            arch = "x86_64";
          };
          tarball-aarch64-darwin = mkTarball {
            inherit pkgs;
            dedupify = dedupify-aarch64-darwin;
            goos = "darwin";
            arch = "aarch64";
          };

          download = tarball: "${repo}/releases/download/v${version}/${tarball.name}";

          # Everything a GitHub release needs, plus the Homebrew formula that
          # points at it. The formula is generated here, from the same
          # tarballs, so its sha256 values cannot drift from what is uploaded.
          release = pkgs.runCommand "dedupify-${version}-release" { } ''
            mkdir -p "$out"
            # Copy under each derivation's `name` rather than `cp X "$out/"`,
            # which would keep the store-hash prefix and leak it into the
            # published filenames.
            cp ${tarball-aarch64-linux} "$out/${tarball-aarch64-linux.name}"
            cp ${tarball-x86_64-linux} "$out/${tarball-x86_64-linux.name}"
            cp ${tarball-aarch64-darwin} "$out/${tarball-aarch64-darwin.name}"
            ( cd "$out" && ${pkgs.buildPackages.coreutils}/bin/sha256sum *.tar.gz > SHA256SUMS )

            sha() { ${pkgs.buildPackages.coreutils}/bin/sha256sum "$out/$1" | cut -d' ' -f1; }
            cat > "$out/dedupify.rb" <<EOF
            class Dedupify < Formula
              desc "Find duplicate files by content hash"
              homepage "${repo}"
              version "${version}"

              on_macos do
                depends_on arch: :arm64

                url "${download tarball-aarch64-darwin}"
                sha256 "$(sha ${tarball-aarch64-darwin.name})"
              end

              on_linux do
                on_arm do
                  url "${download tarball-aarch64-linux}"
                  sha256 "$(sha ${tarball-aarch64-linux.name})"
                end
                on_intel do
                  url "${download tarball-x86_64-linux}"
                  sha256 "$(sha ${tarball-x86_64-linux.name})"
                end
              end

              def install
                bin.install "dedupify"
              end

              test do
                assert_match version.to_s, shell_output("#{bin}/dedupify --version")
              end
            end
            EOF
          '';
        in
        {
          default = native;
          dedupify = native;
          inherit
            dedupify-aarch64-linux
            dedupify-x86_64-linux
            dedupify-aarch64-darwin
            tarball-aarch64-linux
            tarball-x86_64-linux
            tarball-aarch64-darwin
            release
            ;
        }
      );

      # `nix flake check` runs the tests (via the native build's checkPhase)
      # and proves every release target compiles and the formula generates.
      checks = forAllSystems (system: {
        inherit (self.packages.${system})
          default
          dedupify-aarch64-linux
          dedupify-x86_64-linux
          dedupify-aarch64-darwin
          release
          ;
      });

      devShells = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = pkgs.mkShell {
            packages = [
              pkgs.go
              pkgs.go-task
              pkgs.golangci-lint
              pkgs.gopls
              pkgs.git
            ];
          };
        }
      );

      formatter = forAllSystems (system: nixpkgs.legacyPackages.${system}.nixfmt);
    };
}
