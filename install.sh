#!/bin/sh
# Installs the basekit binary from a GitHub release (macOS, Linux, WSL, Git Bash, MSYS2, Cygwin).
#
#   curl -fsSL https://raw.githubusercontent.com/F4R1DS4NCH3Z26/basekit-go/main/install.sh | sh
#   curl -fsSL https://raw.githubusercontent.com/F4R1DS4NCH3Z26/basekit-go/main/install.sh | sh -s -- --version v0.1.0 --dir ~/bin
#
# Environment:
#   BASEKIT_VERSION       same as --version (default: the latest release)
#   BASEKIT_INSTALL_DIR   same as --dir (default: $HOME/.local/bin)
#   BASEKIT_DOWNLOAD_URL  directory holding the release assets (default:
#                         https://github.com/F4R1DS4NCH3Z26/basekit-go/releases/download/<tag>); file:// works
#   BASEKIT_OS, BASEKIT_ARCH  test-only overrides of `uname -s` and `uname -m`
set -eu

repo="F4R1DS4NCH3Z26/basekit-go"

say() { printf '%s\n' "$*"; }
die() { printf 'basekit: %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<'EOF'
Instala basekit desde un release de GitHub.

Uso: install.sh [--version vX.Y.Z] [--dir <directorio>]

  --version  versión a instalar (por defecto la última; también BASEKIT_VERSION)
  --dir      dónde instalar el binario (por defecto $HOME/.local/bin; también BASEKIT_INSTALL_DIR)
EOF
}

version="${BASEKIT_VERSION:-}"
dir="${BASEKIT_INSTALL_DIR:-}"
while [ $# -gt 0 ]; do
  case "$1" in
    --version) [ $# -ge 2 ] || die "falta el valor de --version"; version=$2; shift 2 ;;
    --version=*) version=${1#*=}; shift ;;
    --dir) [ $# -ge 2 ] || die "falta el valor de --dir"; dir=$2; shift 2 ;;
    --dir=*) dir=${1#*=}; shift ;;
    -h | --help) usage; exit 0 ;;
    *) die "opción desconocida: $1 (usa --help)" ;;
  esac
done

# Platform: release assets are named after GOOS/GOARCH.
os_raw="${BASEKIT_OS:-$(uname -s)}"
arch_raw="${BASEKIT_ARCH:-$(uname -m)}"
case "$os_raw" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  MINGW* | MSYS* | CYGWIN*) os=windows ;;
  *) die "sistema operativo no soportado: $os_raw (soportados: macOS, Linux y Windows con Git Bash, MSYS2 o Cygwin; en PowerShell usa install.ps1)" ;;
esac
case "$arch_raw" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) die "arquitectura no soportada: $arch_raw (soportadas: x86_64/amd64 y arm64/aarch64)" ;;
esac
bin=basekit ext=tar.gz
if [ "$os" = windows ]; then bin=basekit.exe ext=zip; fi

have() { command -v "$1" >/dev/null 2>&1; }

# fetch <url> <file>: downloads with curl, else wget.
fetch() {
  if have curl; then curl -fsSL -o "$2" "$1" 2>/dev/null || die "no se pudo descargar $1"
  elif have wget; then wget -q -O "$2" "$1" || die "no se pudo descargar $1"
  else die "se necesita curl o wget para descargar"
  fi
}

# latest_tag: follows the releases/latest redirect (no GitHub API, so no rate limit).
latest_tag() {
  url="https://github.com/$repo/releases/latest"
  if have curl; then
    final=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$url" 2>/dev/null) || final=""
  elif have wget; then
    final=$(wget -q -S --max-redirect=0 -O /dev/null "$url" 2>&1 | sed -n 's/^ *[Ll]ocation: *//p' | tr -d '\r' | tail -n 1) || final=""
  else
    die "se necesita curl o wget para descargar"
  fi
  case "$final" in
    */releases/tag/*) printf '%s\n' "${final##*/}" ;;
    *) die "no se pudo averiguar la última versión; indícala con --version vX.Y.Z" ;;
  esac
}

if [ -z "$version" ]; then version=$(latest_tag); fi
ver=${version#v}
tag="v$ver"
asset="basekit_${ver}_${os}_${arch}.${ext}"
base="${BASEKIT_DOWNLOAD_URL:-https://github.com/$repo/releases/download/$tag}"
base=${base%/}

dir="${dir:-${HOME:?HOME no está definido}/.local/bin}"
case "$dir" in
  \~) dir=$HOME ;;
  \~/*) dir="$HOME/${dir#\~/}" ;;
esac
case "$dir" in
  /*) ;;
  *) dir="$(pwd)/$dir" ;;
esac
dir=${dir%/}

# Work in a temp dir removed on exit, also on failure; the binary is staged next to its target and renamed.
tmp=""
staged=""
cleanup() {
  if [ -n "$tmp" ]; then rm -rf "$tmp"; fi
  if [ -n "$staged" ]; then rm -f "$staged"; fi
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM
tmp=$(mktemp -d 2>/dev/null || mktemp -d -t basekit) || die "no se pudo crear un directorio temporal"

say "Descargando basekit $tag ($os/$arch)..."
fetch "$base/$asset" "$tmp/$asset"
fetch "$base/checksums.txt" "$tmp/checksums.txt"

want=$(awk -v f="$asset" '$2 == f || $2 == "*" f { print $1; exit }' "$tmp/checksums.txt")
[ -n "$want" ] || die "checksums.txt no tiene $asset; no se instaló nada"
if have sha256sum; then got=$(sha256sum "$tmp/$asset" | awk '{ print $1 }')
elif have shasum; then got=$(shasum -a 256 "$tmp/$asset" | awk '{ print $1 }')
else die "se necesita sha256sum o shasum para verificar la descarga"
fi
[ "$got" = "$want" ] || die "el checksum no coincide para $asset (esperado $want, obtenido $got); no se instaló nada"

if [ "$ext" = zip ]; then
  have unzip || die "se necesita unzip para extraer $asset"
  unzip -q "$tmp/$asset" -d "$tmp/x" || die "no se pudo extraer $asset"
else
  mkdir "$tmp/x"
  tar -xzf "$tmp/$asset" -C "$tmp/x" || die "no se pudo extraer $asset"
fi
src="$tmp/x/basekit_${ver}_${os}_${arch}/$bin"
[ -f "$src" ] || die "$asset no contiene $bin"

mkdir -p "$dir" || die "no se pudo crear $dir"
staged="$dir/.$bin.tmp.$$"
cp "$src" "$staged" || die "no se pudo escribir en $dir"
chmod 755 "$staged"
mv -f "$staged" "$dir/$bin" || die "no se pudo reemplazar $dir/$bin"
staged=""

installed=$("$dir/$bin" --version 2>&1) || die "basekit se instaló en $dir/$bin pero no se pudo ejecutar: $installed"
say "basekit $installed instalado en $dir/$bin"

case ":$PATH:" in
  *":$dir:"* | *":$dir/:"*) ;;
  *)
    say ""
    say "$dir no está en tu PATH. Agrega esta línea a tu ~/.profile, ~/.bashrc o ~/.zshrc y abre una terminal nueva:"
    say "  export PATH=\"$dir:\$PATH\""
    ;;
esac
