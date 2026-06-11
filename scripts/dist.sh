#!/bin/sh
# dist.sh — build the six CGO-free release archives into dist/.
#
# Usage: scripts/dist.sh [version]
# Version defaults to `git describe`; release builds pass the tag.
set -eu

cd "$(dirname "$0")/.."
version=${1:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
ldflags="-s -w -X github.com/mjbarefo/loopy/internal/loop.Version=$version"

rm -rf dist
mkdir -p dist

echo "==> building loopy $version"
for target in "darwin amd64" "darwin arm64" "linux amd64" "linux arm64" "windows amd64" "windows arm64"; do
  set -- $target
  GOOS=$1 GOARCH=$2
  bin=loopy
  [ "$GOOS" = windows ] && bin=loopy.exe
  out="dist/$GOOS-$GOARCH"
  mkdir -p "$out"
  echo "    $GOOS/$GOARCH"
  CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build -trimpath -ldflags "$ldflags" -o "$out/$bin" ./cmd/loopy
  cp README.md "$out/"
  name="loopy_${version}_${GOOS}_${GOARCH}"
  if [ "$GOOS" = windows ]; then
    (cd "$out" && zip -q "../$name.zip" "$bin" README.md)
  else
    tar -czf "dist/$name.tar.gz" -C "$out" "$bin" README.md
  fi
  rm -rf "$out"
done

(cd dist && shasum -a 256 ./* | sed 's|\./||' > SHA256SUMS)
echo "==> dist/ ready:"
ls -1 dist
