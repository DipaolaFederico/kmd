#!/usr/bin/env bash
# stamp-version.sh VERSION DATE — write a release version (X.Y.Z) and date
# (YYYY-MM-DD) into the files that carry it: the RPM spec, the man page and
# the AppStream metainfo (a <release> entry is added if missing). Called by
# scripts/sync-version.sh, once per release; `make packaging` checks that
# the three files agree.
set -euo pipefail

ver=$1 date=$2
[[ $ver =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "stamp-version: bad version '$ver'" >&2; exit 2; }
[[ $date =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]] || { echo "stamp-version: bad date '$date'" >&2; exit 2; }
root=$(cd "$(dirname "$0")/.." && pwd)
spec=$root/packaging/kmd.spec
man=$root/docs/kmd.1
meta=$root/packaging/io.github.dipaolafederico.kmd.metainfo.xml

sed -i -E "s/^(Version:[[:space:]]+).*/\1$ver/" "$spec"
sed -i -E "1s/^\.TH KMD 1 \"[^\"]*\" \"kmd [^\"]*\"/.TH KMD 1 \"$date\" \"kmd $ver\"/" "$man"
if ! grep -q "<release version=\"$ver\"" "$meta"; then
  sed -i "s|<releases>|<releases>\n    <release version=\"$ver\" date=\"$date\"/>|" "$meta"
fi

grep -q "^Version:[[:space:]]*$ver\$" "$spec" && grep -q "\"kmd $ver\"" "$man" ||
  { echo "stamp-version: stamping failed" >&2; exit 1; }
echo "stamped $ver ($date)"
