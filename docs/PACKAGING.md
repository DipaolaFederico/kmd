# Distributing kmd on Fedora

The goal is `sudo dnf install kmd` on Fedora. There are two stages:

1. **COPR** (a Fedora-hosted package repository, no review): users run
   `sudo dnf copr enable fdipa/kmd` once, then `dnf install kmd`.
   Packit builds every release there automatically.
2. **Official Fedora repositories**: plain `sudo dnf install kmd`, and kmd
   is listed in the AppStream catalog. This needs a package review and a
   Fedora packager account.

## What is ready in this repository

- `packaging/kmd.spec` follows the Fedora Go packaging conventions as
  produced by `go2rpm` 1.19: `%gometa -L -f` (no `%ix86`), Go modules mode,
  `%gobuild`, `%gotest ./...` in `%check` under `%bcond check`,
  `%autorelease`/`%autochangelog`. kmd has **no Go dependencies**, so there
  is no vendor tarball and no `bundled()` provides; the License is MIT.
- Man page, bash/zsh/fish completions and AppStream metadata
  (`console-application`, with a screenshot) are installed.
- `%check` runs the whole test suite, including the CommonMark/GFM
  specification, and validates the AppStream file. Tests that bound the
  time of hostile inputs are scaled with `KMD_TEST_TIME_SCALE=10`, so slow
  builders (s390x, ppc64le) do not fail them.
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
version stamped into the spec, man page and AppStream metadata.

## Building the RPM locally

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

## Stage 1: COPR with Packit (manual steps, once)

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

## Stage 2: official Fedora repositories (manual steps)

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
