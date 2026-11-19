#!/bin/bash
# Builds the Slackware package and a ready-to-publish drobo.plg.
#   build/package.sh 2026.11.19        -> dist/drobo-2026.11.19-x86_64-1.txz, dist/drobo.plg
set -euo pipefail
VERSION=${1:?usage: build/package.sh YYYY.MM.DD}
cd "$(dirname "$0")/.."
ROOT=$(mktemp -d)
trap 'rm -rf "$ROOT"' EXIT
mkdir -p dist "$ROOT/usr/local/bin" "$ROOT/install"

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags "-s -w -X main.version=$VERSION" -o "$ROOT/usr/local/bin/drobo-status" ./cmd/drobo-status
cp -r src/. "$ROOT/"
cp LICENSE NOTICE "$ROOT/usr/local/emhttp/plugins/drobo/"
cat > "$ROOT/install/slack-desc" <<DESC
drobo: drobo (Drobo status for Unraid)
drobo:
drobo: Shows Drobo bay status, capacity and health on the Unraid Main tab,
drobo: and sends notifications when something changes. Read-only.
drobo:
drobo: https://github.com/yeixio/unraid-drobo
drobo:
DESC
# mktemp makes the root 0700; the package root becomes / on install, so it must be 0755.
chmod 0755 "$ROOT"
find "$ROOT" -type d -exec chmod 0755 {} +
chmod 0755 "$ROOT/usr/local/bin/drobo-status" "$ROOT/usr/local/emhttp/plugins/drobo/scripts/"*

PKG="drobo-$VERSION-x86_64-1.txz"
tar -C "$ROOT" --owner=0 --group=0 --sort=name -cJf "dist/$PKG" .
MD5=$(md5sum "dist/$PKG" | cut -d' ' -f1)
sed -e "s|<!ENTITY version   \"[^\"]*\">|<!ENTITY version   \"$VERSION\">|" \
    -e "s|<!ENTITY md5       \"[^\"]*\">|<!ENTITY md5       \"$MD5\">|" plugin/drobo.plg > dist/drobo.plg
echo "dist/$PKG  md5 $MD5"
