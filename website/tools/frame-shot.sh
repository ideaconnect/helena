#!/usr/bin/env bash
# frame-shot.sh — dress a Helena screenshot as a floating macOS-style window
# (title bar with traffic lights, rounded corners, soft drop shadow on a
# transparent background) for the website hero box. Requires ImageMagick.
#
#   website/tools/frame-shot.sh IN.png OUT.png
#
# Regenerate the hero-box art with `make screenshots-fancy` from the repo root.
set -euo pipefail
src="$1"; out="$2"
W=$(identify -format '%w' "$src")
H=$(identify -format '%h' "$src")
BAR=84      # title-bar height (px, at the 2x capture scale)
RAD=26      # corner radius
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# 1) title bar with three traffic-light dots
convert -size "${W}x${BAR}" xc:'#0f1620' \
  -fill '#ff5f57' -draw 'circle 64,42 64,28' \
  -fill '#febc2e' -draw 'circle 116,42 116,28' \
  -fill '#28c840' -draw 'circle 168,42 168,28' \
  "$tmp/bar.png"

# 2) stack the bar on top of the screenshot
convert "$tmp/bar.png" "$src" -append "$tmp/win.png"

# 3) round the window corners (white round-rect on black = the opacity mask)
WH=$((H + BAR))
convert -size "${W}x${WH}" xc:black -fill white \
  -draw "roundrectangle 0,0,$((W-1)),$((WH-1)),${RAD},${RAD}" "$tmp/mask.png"
convert "$tmp/win.png" "$tmp/mask.png" -alpha off -compose CopyOpacity -composite "$tmp/rounded.png"

# 4) soft drop shadow on a transparent canvas, then scale down for the web
convert "$tmp/rounded.png" \
  \( +clone -background black -shadow 50x35+0+22 \) +swap \
  -background none -layers merge +repage -resize 1600x "$out"

echo "wrote $out"
