#!/usr/bin/env bash
# package-deb.sh VERSION ARCH BINARY HOSTBIN OUTDIR — build the Debian/Ubuntu
# package OUTDIR/kmd_VERSION_ARCH.deb from a prebuilt linux BINARY for ARCH
# (amd64 or arm64). HOSTBIN is a kmd binary that runs on this machine: it
# prints the shell completions, which are the same for every architecture.
# Needs dpkg-deb. The output is reproducible (fixed owners and mtimes).
set -euo pipefail

ver=$1 arch=$2 bin=$3 hostbin=$4 out=$5
[[ $ver =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "package-deb: bad version '$ver'" >&2; exit 2; }
case $arch in amd64 | arm64) ;; *) echo "package-deb: bad arch '$arch'" >&2; exit 2 ;; esac
root=$(cd "$(dirname "$0")/.." && pwd)
for f in "$bin" "$hostbin" "$root/docs/kmd.1" "$root/LICENSE"; do
  [ -f "$f" ] && [ ! -L "$f" ] || { echo "package-deb: $f must be a regular file" >&2; exit 1; }
done

# dpkg-deb dates the archive members with it: fixed, so builds are identical
export SOURCE_DATE_EPOCH=${SOURCE_DATE_EPOCH:-1767225600}
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
pkg=$stage/kmd_${ver}_$arch
umask 022

install -Dm0755 "$bin" "$pkg/usr/bin/kmd"
install -d "$pkg/usr/share/man/man1" "$pkg/usr/share/doc/kmd"
gzip -9n <"$root/docs/kmd.1" >"$pkg/usr/share/man/man1/kmd.1.gz"
"$hostbin" --completion bash | install -Dm0644 /dev/stdin "$pkg/usr/share/bash-completion/completions/kmd"
"$hostbin" --completion zsh | install -Dm0644 /dev/stdin "$pkg/usr/share/zsh/vendor-completions/_kmd"
"$hostbin" --completion fish | install -Dm0644 /dev/stdin "$pkg/usr/share/fish/vendor_completions.d/kmd.fish"

# machine-readable copyright (DEP-5) with the MIT license text
{
  cat <<'EOF'
Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/
Upstream-Name: kmd
Source: https://github.com/DipaolaFederico/kmd

Files: *
Copyright: Federico Di Paola
License: MIT

License: MIT
EOF
  sed -e 's/^[[:space:]]*$/./' -e 's/^/ /' "$root/LICENSE"
} >"$pkg/usr/share/doc/kmd/copyright"
date=$(LC_ALL=C date -u -d "@$SOURCE_DATE_EPOCH" '+%a, %d %b %Y %H:%M:%S +0000')
printf 'kmd (%s) unstable; urgency=medium\n\n  * Release %s: https://github.com/DipaolaFederico/kmd/releases/tag/v%s\n\n -- Federico Di Paola <65874112+DipaolaFederico@users.noreply.github.com>  %s\n' \
  "$ver" "$ver" "$ver" "$date" | gzip -9n >"$pkg/usr/share/doc/kmd/changelog.gz"

install -d "$pkg/DEBIAN"
(cd "$pkg" && find usr -type f -print0 | LC_ALL=C sort -z | xargs -0 md5sum) >"$pkg/DEBIAN/md5sums"
size=$(du -sk --apparent-size "$pkg/usr" | cut -f1)
cat >"$pkg/DEBIAN/control" <<EOF
Package: kmd
Version: $ver
Architecture: $arch
Maintainer: Federico Di Paola <65874112+DipaolaFederico@users.noreply.github.com>
Installed-Size: $size
Section: text
Priority: optional
Homepage: https://github.com/DipaolaFederico/kmd
Description: Fast, beautiful Markdown previewer for the terminal
 kmd renders Markdown files as rich, colorful previews directly in the
 terminal: CommonMark and GitHub Flavored Markdown, syntax highlighting,
 clickable hyperlinks, inline images, Mermaid and Graphviz diagrams, LaTeX
 math rendered with Unicode, and customizable color themes.
 .
 It has a built-in viewer, a table of contents, live preview, a
 presentation mode and HTML export, in a single binary without
 dependencies that starts in a few milliseconds.
EOF
chmod 0644 "$pkg/DEBIAN/control" "$pkg/DEBIAN/md5sums"
find "$pkg" -exec touch -h -d "@$SOURCE_DATE_EPOCH" {} +

mkdir -p "$out"
dpkg-deb --root-owner-group -Zxz --build "$pkg" "$out/kmd_${ver}_$arch.deb" >/dev/null
echo "$out/kmd_${ver}_$arch.deb"
