#!/usr/bin/env sh
# Installs docsmaker for the current user (no root needed).
# Run from the extracted release folder:  ./install.sh
#
# It copies docsmaker to ~/.local/bin. Nothing else is changed and nothing is
# downloaded. If ~/.local/bin is not on your PATH it tells you how to add it.
set -eu

here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/docsmaker" ] || { echo "docsmaker not found next to this script." >&2; exit 1; }

dest="${HOME}/.local/bin"
mkdir -p "$dest"
install -m 755 "$here/docsmaker" "$dest/docsmaker"
echo "installed $dest/docsmaker"

case ":${PATH}:" in
  *":${dest}:"*) ;;
  *) echo "add it to your PATH:  export PATH=\"$dest:\$PATH\"   (put this in your shell profile)" ;;
esac

echo
echo "Next steps:"
echo "  docsmaker model pull      # installs/starts Ollama and downloads the model (asks first)"
echo "  docsmaker --help"
