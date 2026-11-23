# Distributing kmd

Every release is published to these channels by
[`.github/workflows/release.yml`](../.github/workflows/release.yml), with no
manual steps once the one-time setup below is done:

| Channel | What users run | Updated by |
|---------|----------------|------------|
| GitHub release | archives, `.deb`, `SHA256SUMS`, provenance | `build` + `publish` jobs |
| Debian/Ubuntu | `sudo apt install ./kmd_*.deb` | `.deb` attached to the release |
| Homebrew | `brew install DipaolaFederico/tap/kmd` | `tap` job → `DipaolaFederico/homebrew-tap` |
| Scoop | `scoop bucket add dipaolafederico https://github.com/DipaolaFederico/scoop-bucket` · `scoop install kmd` | `tap` job → `DipaolaFederico/scoop-bucket` |
| AUR (pending: see below) | `yay -S kmd-bin` | `aur` job → `aur.archlinux.org/kmd-bin.git` |
| winget | `winget install DipaolaFederico.kmd` | `winget` job → pull request to `microsoft/winget-pkgs` |
| Nix | `nix run github:DipaolaFederico/kmd` | `flake.nix` in this repository (built from source) |
| Fedora COPR | `sudo dnf copr enable fdipa/kmd` · `sudo dnf install kmd` | Packit (see [Fedora](#fedora)) |

## How a release reaches the package managers

1. The `build` job (no secrets, read-only token) builds the archives and
   the two `.deb` packages (`scripts/package-deb.sh`) and writes
   `SHA256SUMS`.
2. The `publish` job attests the archives, the `.deb` files and
   `SHA256SUMS`, tags a commit that adds the version files the build
   stamped (RPM spec, man page, AppStream metadata) on top of the released
   commit, and creates the GitHub release.
3. The `tap`, `aur` and `winget` jobs run only after the release exists.
   Each downloads the release's `SHA256SUMS`, checks it is the file the
   `publish` job released (same SHA-256) and that it was attested by this
   workflow (`gh attestation verify --signer-workflow`),
   and generates its manifests with `scripts/gen-manifests.sh` from the
   released commit: the Homebrew formula, the AUR `PKGBUILD` and
   `.SRCINFO`, the Scoop manifest or the three winget manifests, every
   download pinned by its SHA-256. Nothing the build job wrote other than
   the attested checksums reaches a package repository, and only the
   expected files are copied. These jobs hold the publishing credentials
   (environment **`packages`**); a channel whose secret is missing is
   skipped with a warning, so the release itself never depends on them.
4. The `version-pr` job (also in **`packages`**) stamps the same versions
   into `main`'s files with `scripts/sync-version.sh`, pushes them to the
   bot's fork and opens the pull request `chore(release): vX.Y.Z` from
   there, so the repository always shows the latest release. The
   maintainer reviews and merges it; it publishes nothing.

`make packaging` (part of `make ci`) validates the same output from a
made-up `SHA256SUMS`: Ruby syntax of the formula, `.SRCINFO` against the
`PKGBUILD` (and `makepkg --printsrcinfo` when available), JSON and YAML of
the Windows manifests, and the `.deb` contents, ownership,
reproducibility and `lintian`. Locally missing validators are skipped; CI
runs it with `STRICT=1`. The Nix flake is built and tested by the CI `nix`
job (`nix flake check`).

## One-time setup

In *Settings → Environments*, create the environment **`packages`**,
limited to the `main` branch (*Deployment branches and tags → Selected
branches*), and add these secrets to it (not as repository secrets). The
branch rule is what protects the secrets: a workflow edited on another
branch can drop any check written in the workflow itself, but cannot enter
this environment.

| Secret | What it is |
|--------|------------|
| `PACKAGES_TOKEN` | Fine-grained token, *Only select repositories*: `homebrew-tap` and `scoop-bucket`; permission *Contents: Read and write*. |
| `AUR_SSH_KEY` | Private key (ed25519, no passphrase) whose public key is in the AUR account's *SSH Public Key*. |
| `BOT_TOKEN` | Classic token with only the `public_repo` and `workflow` scopes **of a bot account** (fine-grained tokens cannot open pull requests in other owners' repositories; `workflow` because the version branch pushed to the bot's fork carries the commits since the fork was last updated, and GitHub rejects a push that changes `.github/workflows/` without it). It can write to every public repository *of the bot*, which owns nothing but two forks: `<bot>/kmd` (version pull requests) and `<bot>/winget-pkgs` (winget pull requests). The bot must **not** be a collaborator of `kmd`: then the token cannot push tags, branches or releases here, and the owner can approve its pull requests (a pull request opened with the workflow's own token would not run CI; one opened by the owner could not be approved by the owner). |

Then, once per channel:

- **Homebrew**: create the public repository `DipaolaFederico/homebrew-tap`
  with a `README.md` (the workflow adds `Formula/kmd.rb`). Users get
  updates with `brew upgrade`; `brew install DipaolaFederico/tap/kmd` taps it
  automatically. Submitting to homebrew-core requires notability
  thresholds (stars, forks); revisit after 1.0.
- **Scoop**: create the public repository `DipaolaFederico/scoop-bucket`
  with a `README.md` (the workflow adds `bucket/kmd.json`). The manifest
  has no `checkver`/`autoupdate` on purpose: only the release workflow,
  from attested checksums, updates it.
- **AUR**: create an account at <https://aur.archlinux.org/register> and add
  the public key of `AUR_SSH_KEY`. The first push creates the package
  `kmd-bin`; nothing else to do. *Pending:* AUR registration was paused
  in November 2026, so `AUR_SSH_KEY` is not set yet and the `aur` job is
  skipped; once the account exists, generate the key
  (`ssh-keygen -t ed25519 -N ''`), store the private key with
  `gh secret set AUR_SSH_KEY --env packages < key`, add the README row and
  publish with *Actions → Release → Run workflow*. To test a PKGBUILD by hand, generate it
  (`scripts/gen-manifests.sh X.Y.Z DATE SHA256SUMS out/`) and run
  `makepkg -si` in `out/aur`.
- **Bot account** (version pull requests and winget): create a GitHub
  account (e.g. `kmd-release-bot`); with it, fork
  `DipaolaFederico/kmd` and <https://github.com/microsoft/winget-pkgs>,
  and create `BOT_TOKEN`. Do not add it as a collaborator. The jobs find
  the forks from the token's login. The bot's first pull request to `kmd`
  needs *Approve and run* for CI (first-time contributor); later ones run
  directly.
- **winget**: each release opens a pull request from the bot's fork (the
  first one as *New package*); the winget moderators merge it, which can
  take a few days. On the first submission sign the Microsoft CLA when the
  bot asks.
- **Debian/Ubuntu**: nothing; the `.deb` files are release assets.
- **Nix**: nothing for the flake. `flake.lock` pins nixpkgs; update it with
  `nix flake update` from time to time. For nixpkgs, open a pull request
  adding `pkgs/by-name/km/kmd/package.nix` (a `buildGoModule` like the
  flake's, from the release tag with `vendorHash = null` and the version in
  `ldflags`), following the
  [nixpkgs contributing guide](https://github.com/NixOS/nixpkgs/blob/master/CONTRIBUTING.md).
  Its merge is not required for 1.0.

The flake builds whatever commit it is pointed at, so `kmd --version` from
the flake prints `0-unstable-DATE-COMMIT`. `nix run
github:DipaolaFederico/kmd/vX.Y.Z` builds exactly the released source.

## Testing a channel without a release

`scripts/gen-manifests.sh` and `scripts/package-deb.sh` run anywhere. With
archives built like the release workflow does (see its *Build binaries*
step) and their `SHA256SUMS`, the generated files can be tried in
containers: `brew install` from a local tap in `homebrew/brew` (point the
`url`s at `file://`), `makepkg -si` in `archlinux` with the archive next to
the `PKGBUILD`, `apt install ./kmd_*.deb` in `ubuntu`, and `nix build` in
`nixos/nix`.

## Fedora

The goal is `sudo dnf install kmd` on Fedora. There are two stages:

1. **COPR** (a Fedora-hosted package repository, no review): users run
   `sudo dnf copr enable fdipa/kmd` once, then `dnf install kmd`.
   Packit builds every release there automatically.
2. **Official Fedora repositories**: plain `sudo dnf install kmd`, and kmd
   is listed in the AppStream catalog. This needs a package review and a
   Fedora packager account.

### What is ready in this repository

- `packaging/kmd.spec` follows the Fedora Go packaging conventions as
  produced by `go2rpm` 1.19: `%gometa -L -f` (no `%ix86`), Go modules mode,
  `%gobuild`, `%gotest ./...` in `%check` under `%bcond check`,
  `%autorelease`/`%autochangelog`. kmd has **no Go dependencies**, so there
  is no vendor tarball and no `bundled()` provides; the License is MIT.
- Man page, bash/zsh/fish completions and AppStream metadata
  (`console-application`, with a screenshot) are installed.
- `%check` runs the whole test suite, including the CommonMark/GFM
  specification, and validates the AppStream file. Tests that bound the
  time of hostile inputs are scaled with `KMD_TEST_TIME_SCALE=50`, so slow
  builders (riscv64, s390x, ppc64le) do not fail them.
- Verified: `rpmbuild` in clean `fedora:45` and `fedora:rawhide`
  containers (tests pass, the package installs and runs), `rpmlint` with
  0 errors and 0 warnings, the code and tests cross-compile for aarch64,
  ppc64le, s390x and riscv64.
- `.packit.yaml`: Packit builds each GitHub release in COPR for every
  supported Fedora release, and test-builds every pull request.
- `packaging/packit-distgit.yaml`: once kmd is in Fedora, the same
  automation for the official package (dist-git pull request, Koji build,
  Bodhi update for each release).

The release tarball (`kmd-X.Y.Z.tar.gz`, attached to every GitHub release
by `.github/workflows/release.yml`) is the spec's `Source`; it carries the
version stamped into the spec, man page and AppStream metadata, as does
the tag itself (so Packit's `git archive` of the tag has them too).

### Building the RPM locally

```sh
sudo dnf install rpm-build go-rpm-macros libappstream-glib rpmlint
make rpm                          # rpmbuild + rpmlint from the working tree
```

For a build in a clean chroot, like Fedora's builders:

```sh
sudo dnf install mock fedora-review
sudo usermod -aG mock $USER       # then log in again
make srpm
mock -r fedora-rawhide-x86_64 dist/rpmbuild/SRPMS/kmd-*.src.rpm
```

### Stage 1: COPR with Packit (manual steps, once)

1. Create a Fedora account (FAS) at <https://accounts.fedoraproject.org>.
   The COPR owner in `.packit.yaml` (`owner: fdipa`) is the FAS user
   name.
2. Log in at <https://copr.fedorainfracloud.org> with that account (this
   creates your COPR user) and create a project named **kmd**:
   - Chroots: every `fedora-*` release (x86_64 and aarch64).
   - Under *Settings → Integrations*, allow the user **packit** to build in
     the project (or add `packit` with *builder* permission).
3. Install the Packit GitHub app on the repository:
   <https://github.com/apps/packit-as-a-service>, then link your FAS account
   to your GitHub account as Packit asks in its first comment (one-time
   approval: <https://packit.dev/docs/guide/#2-approval>).
4. The next published release is built in COPR automatically. Users then
   install kmd with:

   ```sh
   sudo dnf copr enable fdipa/kmd
   sudo dnf install kmd
   ```

### Stage 2: official Fedora repositories (manual steps)

1. **Join the packager group.** New packagers need a *sponsor*: read
   <https://docs.fedoraproject.org/en-US/package-maintainers/How_to_Get_Sponsored_into_the_Packager_Group/>,
   introduce yourself on <https://discussion.fedoraproject.org> (tag
   `package-review`) and ask for sponsorship, mentioning the review request
   below. Reviewing someone else's package helps.
2. **Open the review request.** In Bugzilla (<https://bugzilla.redhat.com>,
   product *Fedora*, component *Package Review*), title
   `Review Request: kmd - Fast, beautiful Markdown previewer for the terminal`,
   with the URLs of the spec and SRPM: the COPR build of stage 1 provides
   both (or use <https://fedorapeople.org>). Run
   `fedora-review -b <bug number>` and attach the result.
3. **After approval:**
   ```sh
   fedpkg request-repo kmd <bug number>
   fedpkg request-branch --repo kmd f45      # and other active releases
   fedpkg clone kmd && cd kmd
   fedpkg import <the reviewed SRPM> && git commit && git push
   fedpkg build                              # rawhide; then each branch
   fedpkg update                             # branched releases (Bodhi)
   ```
4. **Automate updates.** Commit `packaging/packit-distgit.yaml` to the
   dist-git repository as `.packit.yaml`. In
   <https://release-monitoring.org> add the project (backend *GitHub*,
   `DipaolaFederico/kmd`, version prefix `v`) with a mapping to the Fedora
   package `kmd`, and enable *Monitoring* on
   <https://src.fedoraproject.org/rpms/kmd>. From then on every GitHub
   release becomes a dist-git pull request, a Koji build and a Bodhi
   update without manual work.

Once in the official repositories kmd appears in the AppStream catalog
(as a console application, it is found by searching in software centers
that show command-line tools, such as KDE Discover; GNOME Software focuses
on graphical applications).
