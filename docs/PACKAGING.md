# Packaging status

Where kmd can be installed from, what is still pending, and what to do
next. Every release updates the channels marked *automatic* with no manual
steps; how the release workflow does it is documented in
[`.github/workflows/release.yml`](../.github/workflows/release.yml).

*Last updated: 2026-11-23, release v0.11.0.*

## Channels

| Channel | Install | Status |
|---------|---------|--------|
| GitHub release | archives, `.deb`, `SHA256SUMS` (with provenance) | ✅ automatic |
| Debian/Ubuntu | `sudo apt install ./kmd_*.deb` | ✅ automatic (release asset) |
| Homebrew | `brew install DipaolaFederico/tap/kmd` | ✅ automatic |
| Scoop | `scoop bucket add dipaolafederico https://github.com/DipaolaFederico/scoop-bucket` · `scoop install kmd` | ✅ automatic |
| Nix flake | `nix run github:DipaolaFederico/kmd` | ✅ builds from source, tested in CI |
| Fedora COPR | `sudo dnf copr enable fdipa/kmd` · `sudo dnf install kmd` | ✅ automatic (Packit); v0.11.0 built for Fedora 43, 44, 45 and rawhide, x86_64 only |
| winget | `winget install DipaolaFederico.kmd` | ⏳ first submission waiting: [microsoft/winget-pkgs#487317](https://github.com/microsoft/winget-pkgs/pull/487317) needs the bot to sign the Microsoft CLA |
| AUR | `yay -S kmd-bin` | ⏸️ blocked: AUR registration is paused ([#27](https://github.com/DipaolaFederico/kmd/issues/27)) |
| nixpkgs | `nix-env -iA nixpkgs.kmd` | 📝 not submitted yet ([#30](https://github.com/DipaolaFederico/kmd/issues/30)) |
| Fedora (official) | `sudo dnf install kmd` | 📝 not submitted yet; the spec is ready |
| homebrew-core | `brew install kmd` | 📝 after 1.0, once kmd meets Homebrew's notability thresholds |

For 1.0 ([#23](https://github.com/DipaolaFederico/kmd/issues/23)), every
✅ channel and winget must serve `v1.0.0`; the AUR and nixpkgs are not
required.

## Next steps

### winget

1. Sign the Microsoft CLA from the bot account on
   [#487317](https://github.com/microsoft/winget-pkgs/pull/487317) (reply
   to the CLA bot's comment as it explains).
2. Wait for the winget moderators to merge it (usually a few days).
   Later releases open *New version* pull requests by themselves; an older
   one still waiting for moderation is closed as superseded.

### Fedora COPR

- Decide whether to add the `aarch64` chroots to the COPR project
  (*Settings* of <https://copr.fedorainfracloud.org/coprs/fdipa/kmd/>):
  today only x86_64 is built.

### AUR ([#27](https://github.com/DipaolaFederico/kmd/issues/27))

1. When registration reopens, create an account at
   <https://aur.archlinux.org/register>.
2. Add an SSH key to the account and to the release workflow (see the
   header of `release.yml`).
3. Publish with *Actions → Release → Run workflow* and add the AUR row to
   the README's install table.

### nixpkgs ([#30](https://github.com/DipaolaFederico/kmd/issues/30))

Open a pull request to nixpkgs adding `pkgs/by-name/km/kmd/package.nix`
(a `buildGoModule` like the flake's, built from the release tag), following
the [nixpkgs contributing guide](https://github.com/NixOS/nixpkgs/blob/master/CONTRIBUTING.md).

### Official Fedora repositories

The package is ready for review: `packaging/kmd.spec` follows the Fedora Go
packaging guidelines, passes `rpmlint` with no warnings and builds and
tests cleanly in `fedora:45` and rawhide. What is left is manual:

1. **Get sponsored into the packager group**: introduce yourself on
   <https://discussion.fedoraproject.org> (tag `package-review`) and ask for
   a sponsor, following the
   [sponsorship guide](https://docs.fedoraproject.org/en-US/package-maintainers/How_to_Get_Sponsored_into_the_Packager_Group/).
2. **Open the review request** in <https://bugzilla.redhat.com> (product
   *Fedora*, component *Package Review*), titled
   `Review Request: kmd - Fast, beautiful Markdown previewer for the terminal`,
   with the spec and SRPM URLs of the COPR build, and attach the output of
   `fedora-review -b <bug number>`.
3. **After approval**, import and build it:
   ```sh
   fedpkg request-repo kmd <bug number>
   fedpkg request-branch --repo kmd f45      # and other active releases
   fedpkg clone kmd && cd kmd
   fedpkg import <the reviewed SRPM> && git commit && git push
   fedpkg build && fedpkg update
   ```
4. **Automate updates**: add `packaging/packit-distgit.yaml` to the
   dist-git repository as `.packit.yaml`, and register kmd on
   <https://release-monitoring.org> (backend *GitHub*, version prefix `v`)
   mapped to the Fedora package `kmd`.

To try the RPM locally: `make rpm`.

## Checking a release

After each release, run *Actions → Verify packages → Run workflow*. It
installs the release on clean runners (`.deb` on Ubuntu, Homebrew on macOS,
Scoop and winget on Windows) and checks the version, rendering, man page and
completions. A channel that is not available yet (winget before its first
merge) is reported as a warning, not a failure.
