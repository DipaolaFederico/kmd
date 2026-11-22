# Distributing kmd on Fedora

kmd is prepared for the official Fedora repositories (what `dnf install kmd`
and GNOME Software use). The package follows the
[Fedora Go Packaging Guidelines](https://docs.fedoraproject.org/en-US/packaging-guidelines/Golang/):

- forge import path (`github.com/DipaolaFederico/kmd`) → `%gometa`,
  `%gobuild`, `%gocheck` macros
- **zero Go dependencies**: no vendoring and no extra `golang-*` packages
- MIT license (SPDX), `%license LICENSE`
- man page (`docs/kmd.1`), bash/zsh/fish completions
- AppStream `console-application` metadata → listed in GNOME Software
- `%check` runs the whole test suite (including the CommonMark spec) and
  validates the AppStream file
- `%autorelease` / `%autochangelog` (rpmautospec, required in dist-git)

## 1. Publish the source (once per release)

```sh
git tag -a v0.2.0 -m "kmd 0.2.0"
git push origin main --tags
```

The spec's `Source` is the tarball GitHub generates for the tag
(`https://github.com/DipaolaFederico/kmd/archive/v0.2.0/kmd-0.2.0.tar.gz`).

## 2. Test the RPM locally

```sh
sudo dnf install rpm-build go-rpm-macros libappstream-glib rpmlint mock fedora-packager
make rpm                         # rpmbuild + rpmlint from the local tree
fedpkg --release f45 mockbuild   # clean chroot build (from dist-git)
```

## 3. COPR (available within hours, no review)

A good way to get users before entering Fedora:

```sh
copr-cli create kmd --chroot fedora-rawhide-x86_64 --chroot fedora-45-x86_64
make srpm
copr-cli build kmd dist/rpmbuild/SRPMS/kmd-*.src.rpm
# users: sudo dnf copr enable DipaolaFederico/kmd && sudo dnf install kmd
```

## 4. Official Fedora repositories

1. Create a FAS account at https://accounts.fedoraproject.org and sign the FPCA.
2. Join the `packager` group: new packages from a new contributor need a
   *sponsor* (https://docs.fedoraproject.org/en-US/package-maintainers/How_to_Get_Sponsored_into_the_Packager_Group/).
3. Open a *Review Request* in Bugzilla (product Fedora, component
   `Package Review`) with the URLs of the `.spec` and the `.src.rpm` (from
   COPR, for example) and the output of `fedora-review -b <bug>`.
4. After approval: `fedpkg request-repo kmd <bug>`, import the SRPM
   (`fedpkg import`), then `fedpkg build` and `fedpkg update` for each branch.

## Release checklist

- [ ] `Version:` in `packaging/kmd.spec`
- [ ] `.TH` in `docs/kmd.1`
- [ ] `<release>` in `packaging/io.github.dipaolafederico.kmd.metainfo.xml`
- [ ] `make ci` (checks that the three versions match)
- [ ] tag `vX.Y.Z` + push
