#!/usr/bin/env bash
# Build + ad-hoc-sign the sandbox validation harness into a runnable .app.
# Run on macOS with Go + the Xcode Command Line Tools installed. No Apple
# Developer account is needed for local sandbox testing (ad-hoc signing is
# enough to activate the sandbox); see README.md.
set -euo pipefail
cd "$(dirname "$0")"

APP="HelenaSandboxHarness.app"
# Ad-hoc signature by default. If app-scope bookmarks misbehave under ad-hoc,
# re-run with a real identity from your keychain, e.g.:
#   SIGN_ID="Apple Development: you@example.com (TEAMID)" ./build.sh
SIGN_ID="${SIGN_ID:--}"

echo "==> go build (CGO_ENABLED=1)"
CGO_ENABLED=1 go build -o harness .

echo "==> assemble ${APP}"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS"
cp Info.plist "$APP/Contents/Info.plist"
mv harness "$APP/Contents/MacOS/harness"

echo "==> codesign (identity: ${SIGN_ID})"
codesign --force \
  --entitlements Helena-Harness.entitlements \
  --sign "$SIGN_ID" \
  "$APP"

echo "==> verify entitlements embedded"
codesign -d --entitlements :- "$APP" 2>/dev/null || codesign -d --entitlements - "$APP" 2>/dev/null || true

cat <<EOF

Built ${APP}.

Run it TWICE from THIS terminal (so you see the output):

  ./${APP}/Contents/MacOS/harness      # 1st run: pick a NEW folder outside ~/Library/Containers
  ./${APP}/Contents/MacOS/harness      # 2nd run: the across-relaunch test (the real question)

Start over from scratch (drops the saved bookmark + container):

  rm -rf ~/Library/Containers/tech.idct.helena.harness
EOF
