#!/usr/bin/env bash
# Builds docsmaker for the machine it runs on and packages it into dist/.
#
# The tree-sitter grammar is C code, so the build needs CGO and a C compiler
# (gcc/clang) and cannot cross-compile: run this on each target OS (CI does).
#
#   VERSION=v1.0.0 scripts/build.sh
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
GOOS="$(go env GOOS)"
GOARCH="$(go env GOARCH)"
NAME="docsmaker_${VERSION}_${GOOS}_${GOARCH}"
OUT="dist/${NAME}"

LDFLAGS="-s -w -X github.com/admiravelhomemdasneves/DocsMakerCLI/internal/cli.Version=${VERSION}"
EXT=""
case "$GOOS" in
  windows) EXT=".exe" ;;
  # Fully static on Linux so one binary runs on any distribution.
  linux) LDFLAGS="${LDFLAGS} -linkmode external -extldflags '-static'" ;;
esac

rm -rf "$OUT" "dist/${NAME}.tar.gz"
mkdir -p "$OUT"

echo "building ${NAME} ..."
CGO_ENABLED=1 go build -trimpath -tags netgo,osusergo -ldflags "$LDFLAGS" -o "${OUT}/docsmaker${EXT}" ./cmd/docsmaker

cp README.md "$OUT/"
cp scripts/install.sh "$OUT/"
chmod +x "$OUT/install.sh"

# Smoke test the artifact before shipping it.
"${OUT}/docsmaker${EXT}" --version

tar -C dist -czf "dist/${NAME}.tar.gz" "$NAME"
(cd dist && { sha256sum "${NAME}.tar.gz" 2>/dev/null || shasum -a 256 "${NAME}.tar.gz"; } > "${NAME}.tar.gz.sha256")
echo "wrote dist/${NAME}.tar.gz"
