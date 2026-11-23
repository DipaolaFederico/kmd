#!/usr/bin/env bash
# check-packages.sh BINARY — validate what the release workflow publishes
# besides the archives: the Homebrew formula, AUR PKGBUILD/.SRCINFO, Scoop
# and winget manifests (from a made-up SHA256SUMS) and the .deb (from BINARY,
# a linux kmd for this machine). Run by `make packaging`.
#
# Validators that are not installed are skipped with a note; with
# STRICT=1 (CI) a missing validator is an error.
set -euo pipefail

bin=$1
root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
fails=0
fail() { echo "  FAIL: $*"; fails=$((fails + 1)); }
have() {
  command -v "$1" >/dev/null && return 0
  if [ "${STRICT:-0}" = 1 ]; then fail "$1 is not installed"; else echo "  (skipped: $1 not installed)"; fi
  return 1
}

ver=1.2.3
# a fake checksum per file: digit i repeated (a…f for 10…15)
i=0
for f in darwin-amd64.tar.gz darwin-arm64.tar.gz freebsd-amd64.tar.gz linux-amd64.tar.gz linux-arm64.tar.gz windows-amd64.zip; do
  printf '%s  kmd-v%s-%s\n' "$(printf "%x" $((i + 10)) | head -c1 | sed 's/./&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&/')" "$ver" "$f"
  i=$((i + 1))
done >"$tmp/SHA256SUMS"
sum() { awk -v f="kmd-v$ver-$1" '$2 == f { print $1 }' "$tmp/SHA256SUMS"; }
"$root/scripts/gen-manifests.sh" $ver 2030-01-02 "$tmp/SHA256SUMS" "$tmp/out" >/dev/null
out=$tmp/out

# every manifest pins the right archive: its URL and its checksum together
pins() { # file platform (expects the URL and the checksum in the file)
  grep -qF "kmd-v$ver-$2" "$1" || fail "$1: no URL for $2"
  grep -qiF "$(sum "$2")" "$1" || fail "$1: no SHA-256 for $2"
}
for p in darwin-amd64.tar.gz darwin-arm64.tar.gz linux-amd64.tar.gz linux-arm64.tar.gz; do pins "$out/homebrew/Formula/kmd.rb" "$p"; done
for p in linux-amd64.tar.gz linux-arm64.tar.gz; do
  pins "$out/aur/.SRCINFO" "$p" # the PKGBUILD URLs use ${pkgver}: compared with .SRCINFO below
  grep -qF "$(sum "$p")" "$out/aur/PKGBUILD" || fail "PKGBUILD: no SHA-256 for $p"
done
pins "$out/scoop/bucket/kmd.json" windows-amd64.zip
pins "$out/winget/manifests/d/DipaolaFederico/kmd/$ver/DipaolaFederico.kmd.installer.yaml" windows-amd64.zip
# each formula url is followed by the checksum of that same archive
while read -r url hash; do
  [ "$hash" = "$(sum "${url##*/kmd-v"$ver"-}")" ] || fail "formula: wrong sha256 after $url"
done < <(awk -F '"' '/^ *url / { u = $2 } /^ *sha256 / { print u, $2 }' "$out/homebrew/Formula/kmd.rb")
if "$root/scripts/gen-manifests.sh" 1.2 2030-01-02 "$tmp/SHA256SUMS" "$tmp/bad" >/dev/null 2>&1; then fail "gen-manifests accepted a bad version"; fi
grep -v windows "$tmp/SHA256SUMS" >"$tmp/partial"
if "$root/scripts/gen-manifests.sh" $ver 2030-01-02 "$tmp/partial" "$tmp/bad" >/dev/null 2>&1; then fail "gen-manifests accepted a missing checksum"; fi

# Homebrew: Ruby syntax
if have ruby; then ruby -c "$out/homebrew/Formula/kmd.rb" >/dev/null || fail "formula: ruby syntax"; fi

# AUR: the PKGBUILD parses, and .SRCINFO says what the PKGBUILD says
bash -n "$out/aur/PKGBUILD" || fail "PKGBUILD: bash syntax"
srcinfo=$(
  cd "$out/aur" && unset pkgname pkgver && set +u
  # shellcheck disable=SC1091 # the PKGBUILD defines the variables printed below
  source ./PKGBUILD
  # shellcheck disable=SC2154
  printf '%s\n' "$pkgname" "$pkgver" "${source_x86_64[0]}" "${sha256sums_x86_64[0]}" "${source_aarch64[0]}" "${sha256sums_aarch64[0]}"
)
want=$(for k in pkgname pkgver source_x86_64 sha256sums_x86_64 source_aarch64 sha256sums_aarch64; do
  awk -F ' = ' -v k="$k" '{ sub(/^\t/, "") } $1 == k { print $2; exit }' "$out/aur/.SRCINFO"
done)
[ "$srcinfo" = "$want" ] || fail ".SRCINFO does not match the PKGBUILD:"$'\n'"$srcinfo"$'\n'"---"$'\n'"$want"
if command -v makepkg >/dev/null; then
  (cd "$out/aur" && makepkg --printsrcinfo) | cmp -s - "$out/aur/.SRCINFO" || fail ".SRCINFO differs from makepkg --printsrcinfo"
fi

# Scoop: valid JSON; winget: valid YAML
if have python3; then
  python3 -I -m json.tool "$out/scoop/bucket/kmd.json" >/dev/null || fail "scoop manifest: invalid JSON"
  if python3 -I -c 'import yaml' 2>/dev/null; then
    for y in "$out/winget/manifests/d/DipaolaFederico/kmd/$ver"/*.yaml; do
      python3 -I -c 'import sys, yaml; d = yaml.safe_load(open(sys.argv[1])); assert d["PackageVersion"] == sys.argv[2], d' "$y" $ver ||
        fail "$y: invalid YAML"
    done
  elif [ "${STRICT:-0}" = 1 ]; then fail "python3 yaml is not installed"; else echo "  (skipped: python3 yaml not installed)"; fi
fi

# Debian: build the package and look inside
if have dpkg-deb; then
  deb=$("$root/scripts/package-deb.sh" $ver amd64 "$bin" "$bin" "$tmp/deb")
  [ "$(dpkg-deb -f "$deb" Package Version Architecture | tr '\n' ' ')" = "Package: kmd Version: $ver Architecture: amd64 " ] ||
    fail ".deb control fields"
  want="./usr/bin/kmd ./usr/share/bash-completion/completions/kmd ./usr/share/doc/kmd/changelog.gz ./usr/share/doc/kmd/copyright ./usr/share/fish/vendor_completions.d/kmd.fish ./usr/share/man/man1/kmd.1.gz ./usr/share/zsh/vendor-completions/_kmd"
  got=$(dpkg-deb -c "$deb" | awk '$1 !~ /^d/ { print $6 }' | LC_ALL=C sort | tr '\n' ' ')
  [ "$got" = "$want " ] || fail ".deb contents: $got"
  dpkg-deb -c "$deb" | awk '{ print $2 }' | grep -qv '^root/root$' && fail ".deb: files not owned by root"
  # reproducible: the same inputs give the same bytes
  again=$("$root/scripts/package-deb.sh" $ver amd64 "$bin" "$bin" "$tmp/deb2")
  cmp -s "$deb" "$again" || fail ".deb is not reproducible"
  if have lintian; then
    # a statically linked Go binary is expected; the version has no Debian revision on purpose
    lintian --fail-on error,warning --suppress-tags statically-linked-binary "$deb" || fail "lintian"
  fi
fi

if [ "$fails" -gt 0 ]; then
  echo "  $fails package check(s) failed"
  exit 1
fi
echo "  packages (Homebrew, AUR, Scoop, winget, .deb): ok"
