#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
VERSION=${VERSION:-dev}
GOARCH=${GOARCH:-arm64}
GOARM=${GOARM:-}
GOMIPS=${GOMIPS:-}
ASSET_ARCH=${ASSET_ARCH:-$GOARCH}
OUT="$ROOT/dist/meowdeck-linux-$ASSET_ARCH"

rm -rf "$OUT"
mkdir -p "$OUT"

(
	cd "$ROOT/web"
	npm ci
	npm run build
)

build_env=(CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH")
[ -n "$GOARM" ] && build_env+=(GOARM="$GOARM")
[ -n "$GOMIPS" ] && build_env+=(GOMIPS="$GOMIPS")

(
	cd "$ROOT"
	env "${build_env[@]}" go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "$OUT/meowdeck" .
)

cp "$ROOT/configs/config.example.json" "$OUT/config.example.json"
cp "$ROOT/packaging/openwrt/init.d/meowdeck" "$OUT/meowdeck.init"
cp "$ROOT/packaging/openwrt/nginx/meowdeck.conf" "$OUT/meowdeck.nginx.conf"
cp "$ROOT/packaging/openwrt/dnsmasq/meowdeck.uci" "$OUT/meowdeck.dnsmasq.uci"
cp "$ROOT/packaging/openwrt/meowdeck-update" "$OUT/meowdeck-update"
cp "$ROOT/packaging/openwrt/meowdeck-configure" "$OUT/meowdeck-configure"
cp "$ROOT/packaging/openwrt/update.conf" "$OUT/update.conf"
cp "$ROOT/scripts/install-openwrt.sh" "$OUT/install.sh"
chmod 0755 "$OUT/meowdeck" "$OUT/meowdeck.init" "$OUT/meowdeck-update" "$OUT/meowdeck-configure" "$OUT/install.sh"

(
	cd "$ROOT/dist"
	tar -czf "meowdeck-linux-$ASSET_ARCH.tar.gz" -C "meowdeck-linux-$ASSET_ARCH" .
	sha256sum "meowdeck-linux-$ASSET_ARCH.tar.gz" >"meowdeck-linux-$ASSET_ARCH.tar.gz.sha256"
)
