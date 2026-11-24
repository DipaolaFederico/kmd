#!/usr/bin/env bash
# docs-images.sh BIN [TAPE ...]
# Regenerates the README images in docs/images/ by running the tapes in
# docs/images/tapes/ (all of them, or the ones given) with VHS. VHS, ttyd,
# ffmpeg and the Chromium that VHS drives are documentation tools only:
# nothing in the build, the tests or the releases needs them.
#
# kmd runs as BIN in a clean environment (env -i, empty configuration), so
# the user's settings and themes never show in the pictures. Screenshots
# are reduced to 256 colors, which halves them with no visible change.
# Afterwards each image must be ≤ 500 KiB and all of them ≤ 4 MiB.
set -euo pipefail

[ $# -ge 1 ] || { echo "usage: $0 BIN [TAPE ...]" >&2; exit 2; }
bin=$(realpath "$1")
shift
cd "$(dirname "$0")/.."
max_each=$((500 * 1024)) max_total=$((4 * 1024 * 1024))

missing() { echo "docs-images: $1 not found (CONTRIBUTING.md, \"README images\")" >&2; exit 1; }
for tool in vhs ttyd ffmpeg fc-list; do
	command -v "$tool" >/dev/null || missing "$tool"
done
# without a browser on PATH, VHS would download an unverified one
command -v chromium-browser >/dev/null || command -v chromium >/dev/null ||
	command -v google-chrome >/dev/null || missing "a Chromium"
# the font in settings.tape: a fallback font changes every image silently
fc-list : family | grep 'JetBrains Mono' >/dev/null || missing "the JetBrains Mono font"

home=$(mktemp -d)
# tapes that only take screenshots still need an Output: a text one, in .out
trap 'rm -rf "$home" docs/images/.out' EXIT
mkdir "$home/bin"
ln -s "$bin" "$home/bin/kmd"

# the picture in images.md is generated, so it carries no licence question
[ -f docs/images/mandelbrot.png ] || ffmpeg -loglevel error -f lavfi \
	-i 'mandelbrot=size=320x200:start_x=-0.75:start_y=0:start_scale=2.6:outer=normalized_iteration_count:inner=black' \
	-frames:v 1 -update 1 docs/images/mandelbrot.png

tapes=("$@")
[ ${#tapes[@]} -gt 0 ] || tapes=(docs/images/tapes/*.tape)
for tape in "${tapes[@]}"; do
	echo "» $tape"
	env -i PATH="$home/bin:$PATH" HOME="$home" XDG_CONFIG_HOME="$home/.config" \
		LANG=C.UTF-8 COLORTERM=truecolor vhs -q "$tape"
	sed -n 's/^Screenshot //p' "$tape" | while read -r png; do
		ffmpeg -y -loglevel error -i "$png" -vf \
			'split[a][b];[a]palettegen=max_colors=256:stats_mode=full[p];[b][p]paletteuse=dither=none' \
			"$home/palette.png"
		mv "$home/palette.png" "$png"
	done
done

total=0 fail=0
for img in $(find docs/images -path docs/images/.out -prune -o -type f ! -name '*.md' ! -name '*.tape' ! -name '*.sh' -print); do
	size=$(wc -c <"$img")
	total=$((total + size))
	if [ "$size" -gt "$max_each" ]; then
		echo "docs-images: $img is $((size / 1024)) KiB (max $((max_each / 1024)) KiB)" >&2
		fail=1
	fi
done
echo "  images: $((total / 1024)) KiB in total"
if [ "$total" -gt "$max_total" ]; then
	echo "docs-images: the images add up to more than $((max_total / 1024 / 1024)) MiB" >&2
	fail=1
fi
exit "$fail"
