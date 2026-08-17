#!/usr/bin/env bash
#
# Package a Linux build of helmdex-desktop as an AppImage.
#
# Usage:
#   ./scripts/desktop/build-appimage.sh <amd64|arm64>
#
# Pre-requisites:
#   - linuxdeploy / linuxdeploy-plugin-gtk    (auto-downloaded to ~/.cache)
#   - appimagetool                            (auto-downloaded to ~/.cache)
#   - libfuse2                                (only required to RUN the
#     produced .AppImage — the build uses --appimage-extract-and-run)
#
# The build/bin/helmdex-desktop-linux-<arch> binary is expected to exist
# (run `task desktop:build:linux:<arch>` first).
#
# Adapted from iterion's packaging script — it encodes hard-won WebKitGTK
# AppImage pitfalls (helper processes, host-only lib pairs).

set -euo pipefail

ARCH="${1:-amd64}"
APPDIR="build/bin/Helmdex.AppDir"
BIN="build/bin/helmdex-desktop-linux-${ARCH}"

case "$ARCH" in
  amd64) APPIMAGE_ARCH="x86_64" ;;
  arm64) APPIMAGE_ARCH="aarch64" ;;
  *) echo "Unknown arch: $ARCH" >&2; exit 1 ;;
esac

if [ ! -x "$BIN" ]; then
  echo "Binary not found at $BIN — run 'task desktop:build:linux:$ARCH' first" >&2
  exit 1
fi

rm -rf "$APPDIR"
mkdir -p "$APPDIR/usr/bin" "$APPDIR/usr/share/applications" "$APPDIR/usr/share/icons/hicolor/256x256/apps"

cp "$BIN" "$APPDIR/usr/bin/helmdex-desktop"
cp build/appicon.png "$APPDIR/usr/share/icons/hicolor/256x256/apps/helmdex-desktop.png"
cp build/appicon.png "$APPDIR/helmdex-desktop.png"
cp build/linux/helmdex.desktop "$APPDIR/usr/share/applications/helmdex-desktop.desktop"
cp build/linux/AppImage/AppRun "$APPDIR/AppRun"
chmod +x "$APPDIR/AppRun"

# Bundle WebKit2GTK helper processes (WebKitWebProcess, WebKitNetworkProcess,
# WebKitGPUProcess + injected-bundle). linuxdeploy-plugin-gtk only copies
# libwebkit2gtk-4.1.so.0; without these out-of-process helpers WebKit loads
# but renders nothing. AppRun sets WEBKIT_EXEC_PATH to the bundled dir.
WEBKIT_HELPER_DIR="$APPDIR/usr/lib/webkit2gtk-4.1"
mkdir -p "$WEBKIT_HELPER_DIR"
WEBKIT_HELPER_SRC=""
for candidate in \
    "/usr/lib/x86_64-linux-gnu/webkit2gtk-4.1" \
    "/usr/lib/aarch64-linux-gnu/webkit2gtk-4.1" \
    "/usr/lib/webkit2gtk-4.1" \
    "/usr/libexec/webkit2gtk-4.1"; do
  if [ -d "$candidate" ]; then
    WEBKIT_HELPER_SRC="$candidate"
    break
  fi
done
if [ -z "$WEBKIT_HELPER_SRC" ]; then
  echo "ERROR: webkit2gtk-4.1 helper directory not found on build host." >&2
  echo "       Install libwebkit2gtk-4.1-0 (Debian/Ubuntu) and retry." >&2
  exit 1
fi
cp -a "$WEBKIT_HELPER_SRC"/. "$WEBKIT_HELPER_DIR/"

# Auto-bootstrap linuxdeploy + plugin-gtk if not on PATH; cache in
# ~/.cache so local rebuilds skip the download.
LINUXDEPLOY_CACHE="$HOME/.cache/helmdex-linuxdeploy-${APPIMAGE_ARCH}"
mkdir -p "$LINUXDEPLOY_CACHE"
LINUXDEPLOY_BIN="$LINUXDEPLOY_CACHE/linuxdeploy"
LINUXDEPLOY_PLUGIN_GTK="$LINUXDEPLOY_CACHE/linuxdeploy-plugin-gtk"
if [ ! -x "$LINUXDEPLOY_BIN" ]; then
  curl -fsSL -o "$LINUXDEPLOY_BIN" \
    "https://github.com/linuxdeploy/linuxdeploy/releases/download/continuous/linuxdeploy-${APPIMAGE_ARCH}.AppImage"
  chmod +x "$LINUXDEPLOY_BIN"
fi
if [ ! -x "$LINUXDEPLOY_PLUGIN_GTK" ]; then
  # Pinned commit: a moving raw.github script in a release job is a
  # supply-chain vector.
  curl -fsSL -o "$LINUXDEPLOY_PLUGIN_GTK" \
    "https://raw.githubusercontent.com/linuxdeploy/linuxdeploy-plugin-gtk/3b67a1d1c1b0c8268f57f2bce40fe2d33d409cea/linuxdeploy-plugin-gtk.sh"
  chmod +x "$LINUXDEPLOY_PLUGIN_GTK"
fi
# linuxdeploy looks up `linuxdeploy-plugin-gtk` on PATH, hence the prepend.
export PATH="$LINUXDEPLOY_CACHE:$PATH"

# Exclude libraries that MUST come from the host: tightly-coupled pairs
# (libgcrypt + libgpg-error) or version-sensitive pairs (libssl +
# libcrypto with the host's CA trust). Bundling libgcrypt from a newer
# build host while libgpg-error stays on the user host fails at runtime
# with "undefined symbol: gpgrt_add_post_log_func".
EXCLUDES=(
  --exclude-library=libgcrypt.so\*
  --exclude-library=libgpg-error.so\*
  --exclude-library=libssl.so\*
  --exclude-library=libcrypto.so\*
)

TARGET_NAME="helmdex-desktop-linux-${ARCH}.AppImage"
ARCH="$APPIMAGE_ARCH" "$LINUXDEPLOY_BIN" --appimage-extract-and-run \
  --appdir "$APPDIR" --plugin gtk --output appimage "${EXCLUDES[@]}"

# linuxdeploy-plugin-gtk re-deploys some host-only libs after the main
# excludelist pass. Strip whatever slipped through by extracting,
# removing, and repacking.
APPIMAGE_GLOB=(./*-"${APPIMAGE_ARCH}.AppImage")
APPIMAGE_FILE="${APPIMAGE_GLOB[0]}"
if [ ! -f "$APPIMAGE_FILE" ]; then
  echo "linuxdeploy did not produce an AppImage matching *-${APPIMAGE_ARCH}.AppImage" >&2
  exit 1
fi

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT
(
  cd "$WORKDIR"
  # AppImages are runtime+squashfs concatenated; --appimage-extract gives
  # a writable copy without needing fuse.
  "$OLDPWD/$APPIMAGE_FILE" --appimage-extract >/dev/null
  for stem in libgcrypt.so libgpg-error.so libssl.so libcrypto.so; do
    find squashfs-root -name "${stem}*" -delete
  done
)

# Repack via appimagetool (linuxdeploy is a bundler, not a packer).
APPIMAGETOOL="$HOME/.cache/helmdex-appimagetool-${APPIMAGE_ARCH}.AppImage"
if [ ! -x "$APPIMAGETOOL" ]; then
  mkdir -p "$(dirname "$APPIMAGETOOL")"
  curl -fsSL -o "$APPIMAGETOOL" \
    "https://github.com/AppImage/appimagetool/releases/download/continuous/appimagetool-${APPIMAGE_ARCH}.AppImage"
  chmod +x "$APPIMAGETOOL"
fi

ARCH="$APPIMAGE_ARCH" "$APPIMAGETOOL" --appimage-extract-and-run \
  "$WORKDIR/squashfs-root" "$TARGET_NAME"

# Drop the intermediate AppImage (with host-only libs still bundled).
[ "$APPIMAGE_FILE" != "./$TARGET_NAME" ] && rm -f "$APPIMAGE_FILE"

echo "Built ${TARGET_NAME}"
