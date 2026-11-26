#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

DEMO="artifacts/demo"
WEBM="$DEMO/webm"
RAW="$DEMO/raw"
PREV="$DEMO/previews"
REPORTS="$DEMO/reports"
FRAMES="$(mktemp -d)"
BIN="$(mktemp -d)"

mkdir -p "$WEBM" "$RAW" "$PREV" "$REPORTS" "webm/testdata"

go build -o "$BIN/alphacheck" ./cmd/alphacheck
go build -o "$BIN/mkfixtures" ./cmd/mkfixtures
"$BIN/mkfixtures" png-frames "$FRAMES"

ffmpeg -hide_banner -loglevel error -y \
  -framerate 12 -i "$FRAMES/%03d.png" \
  -c:v libvpx-vp9 -pix_fmt yuva420p -b:v 500k -auto-alt-ref 0 \
  "$WEBM/alpha-ok.webm"

ffmpeg -hide_banner -loglevel error -y \
  -f lavfi -i "color=black:s=96x64:r=12" \
  -framerate 12 -i "$FRAMES/%03d.png" \
  -filter_complex "[0:v][1:v]overlay=format=auto,format=yuva420p" \
  -c:v libvpx-vp9 -pix_fmt yuva420p -b:v 500k -auto-alt-ref 0 \
  -frames:v 12 \
  "$WEBM/encode-alpha-opaque.webm"

cp "$WEBM/alpha-ok.webm" "webm/testdata/alpha-ok.webm"
"$BIN/mkfixtures" variants "$WEBM/alpha-ok.webm" "$WEBM"

for file in "$WEBM"/*.webm; do
  base="$(basename "$file" .webm)"
  "$BIN/alphacheck" inspect -json -all-frames "$file" > "$REPORTS/$base.json"
done

make_preview() {
  name="$1"
  "$BIN/alphacheck" ivf "$WEBM/$name.webm" "$RAW/$name" >/dev/null
  ffmpeg -hide_banner -loglevel error -y -i "$RAW/$name.color.ivf" \
    -frames:v 1 -f rawvideo -pix_fmt rgba "$RAW/$name.rgba"
  ffmpeg -hide_banner -loglevel error -y -i "$RAW/$name.alpha.ivf" \
    -frames:v 1 -f rawvideo -pix_fmt gray "$RAW/$name.gray"
}

make_preview alpha-ok
make_preview encode-alpha-opaque

"$BIN/alphacheck" composite --json \
  --color "$RAW/alpha-ok.rgba" --alpha "$RAW/alpha-ok.gray" \
  --width 96 --height 64 --crop 8,8,0,0 --require-variation \
  --out "$PREV/alpha-ok" > "$PREV/alpha-ok.metrics.json"

"$BIN/alphacheck" composite --json --double-premultiply \
  --color "$RAW/alpha-ok.rgba" --alpha "$RAW/alpha-ok.gray" \
  --width 96 --height 64 \
  --out "$PREV/composition-double-alpha" > "$PREV/composition-double-alpha.metrics.json"

"$BIN/alphacheck" composite --json \
  --color "$RAW/encode-alpha-opaque.rgba" --alpha "$RAW/encode-alpha-opaque.gray" \
  --width 96 --height 64 --require-variation \
  --out "$PREV/encode-alpha-opaque" > "$PREV/encode-alpha-opaque.metrics.json"

"$BIN/alphacheck" inspect -json -all-frames \
  --preview-metrics "$PREV/alpha-ok.metrics.json" --require-variation \
  "$WEBM/alpha-ok.webm" > "$REPORTS/alpha-ok.json"
"$BIN/alphacheck" inspect -json -all-frames \
  --preview-metrics "$PREV/encode-alpha-opaque.metrics.json" --require-variation \
  "$WEBM/encode-alpha-opaque.webm" > "$REPORTS/encode-alpha-opaque.json"

"$BIN/alphacheck" ivf "$WEBM/encode-corrupt-alpha-header.webm" \
  "$RAW/encode-corrupt-alpha-header" >/dev/null
ffmpeg -hide_banner -v error -y \
  -i "$RAW/encode-corrupt-alpha-header.alpha.ivf" -f null - \
  2> "$PREV/encode-corrupt-alpha.decode-error.log" || true

SUMMARY="$DEMO/summary.txt"
{
  echo "AlphaCheck demo summary"
  echo "Generated from artifacts/demo/webm with pure-Go EBML/frame analysis."
  echo
  for file in "$WEBM"/*.webm; do
    base="$(basename "$file" .webm)"
    echo "==================== $base ===================="
    if [ "$base" = "alpha-ok" ] || [ "$base" = "encode-alpha-opaque" ]; then
      "$BIN/alphacheck" inspect \
        --preview-metrics "$PREV/$base.metrics.json" --require-variation "$file"
    else
      "$BIN/alphacheck" inspect "$file"
    fi
    echo
  done
} > "$SUMMARY"

if command -v magick >/dev/null 2>&1; then
  magick montage -background '#111827' -fill white -pointsize 20 \
    -label 'OK / black background' "$PREV/alpha-ok.black.png" \
    -label 'OK / white background' "$PREV/alpha-ok.white.png" \
    -label 'Alpha=255 / black proof' "$PREV/encode-alpha-opaque.black.png" \
    -label 'Alpha=255 / white proof' "$PREV/encode-alpha-opaque.white.png" \
    -tile 2x2 -geometry 384x256+12+28 "$PREV/contact-sheet.png"

  magick montage -background '#111827' -fill white -pointsize 20 \
    -label 'correct straight alpha / white' "$PREV/alpha-ok.white.png" \
    -label 'double multiply / white' "$PREV/composition-double-alpha.white.png" \
    -tile 2x1 -geometry 384x256+12+28 "$PREV/composition-comparison.png"
fi

echo "demo evidence written to $DEMO"
