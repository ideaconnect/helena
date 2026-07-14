#!/usr/bin/env bash
# og-card.sh — compose the 1200x630 social / OpenGraph card: the framed app
# window centred on Helena's dark background with a soft green accent glow, so
# link previews render at a predictable size instead of cropping a raw
# screenshot. Requires ImageMagick.
#
#   website/tools/og-card.sh IN-fancy.png OUT.png
#
# Regenerate with `make screenshots-fancy` from the repo root.
set -euo pipefail
src="$1"; out="$2"
W=1200; H=630
BG='#0d1117'          # --bg
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# 1) dark base with a soft green glow in the upper-right, matching the site's
#    radial hero glow (rgba(34,197,94,.32), blurred).
convert -size "${W}x${H}" xc:"$BG" "$tmp/base.png"
convert -size "${W}x${H}" xc:none -fill 'rgba(34,197,94,0.32)' \
  -draw 'circle 800,240 800,20' -blur 0x70 "$tmp/glow.png"
convert "$tmp/base.png" "$tmp/glow.png" -composite "$tmp/bg.png"

# 2) the pre-framed app window, scaled to leave breathing room, centred.
convert "$src" -resize x540 "$tmp/win.png"
convert "$tmp/bg.png" "$tmp/win.png" -gravity center -geometry +0+0 -composite "$out"

echo "wrote $out"
