#!/usr/bin/env bash
# Builds the release archives into dist/: one per OS/arch, plus checksums.txt.
# Usage: scripts/build-release.sh <version>   (a leading "v" is dropped)
set -euo pipefail

version="${1:?usage: build-release.sh <version>}"
version="${version#v}"
module="github.com/F4R1DS4NCH3Z26/basekit-go"
targets="windows/amd64 windows/arm64 darwin/amd64 darwin/arm64 linux/amd64 linux/arm64"

rm -rf dist && mkdir -p dist
for target in $targets; do
  os="${target%/*}" arch="${target#*/}"
  name="basekit_${version}_${os}_${arch}"
  bin="basekit"; [ "$os" = windows ] && bin="basekit.exe"
  mkdir -p "dist/$name"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath \
    -ldflags "-s -w -X ${module}/internal/scaffold.Version=${version}" \
    -o "dist/$name/$bin" ./cmd/basekit
  cp README.md "dist/$name/"
  if [ "$os" = windows ]; then
    (cd dist && zip -qr "$name.zip" "$name")
  else
    tar -C dist -czf "dist/$name.tar.gz" "$name"
  fi
  rm -rf "dist/$name"
done

if command -v sha256sum >/dev/null; then sum="sha256sum"; else sum="shasum -a 256"; fi
(cd dist && $sum basekit_* > checksums.txt)
ls -1 dist
