#!/usr/bin/env bash
# Builds nbt-mcp for every supported platform into dist/, with checksums.
# Usage: scripts/build-release.sh v0.1.0
set -euo pipefail

version=${1:?usage: build-release.sh <version>}
root=$(cd "$(dirname "$0")/.." && pwd)
dist=$root/dist
rm -rf "$dist"
mkdir -p "$dist"

cd "$root/cmd/nbt-mcp"
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
  os=${target%/*} arch=${target#*/}
  name=nbt-mcp_${version#v}_${os}_${arch}
  stage=$(mktemp -d)
  bin=nbt-mcp
  [ "$os" = windows ] && bin=nbt-mcp.exe
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath \
    -ldflags "-s -w -X main.buildVersion=$version" -o "$stage/$bin" .
  cp README.md LICENSE "$stage/"
  if [ "$os" = windows ]; then
    (cd "$stage" && zip -q "$dist/$name.zip" ./*)
  else
    tar -C "$stage" -czf "$dist/$name.tar.gz" .
  fi
  rm -rf "$stage"
done

cd "$dist"
sha256sum ./* | sed 's# \./# #' > checksums.txt
ls -l
