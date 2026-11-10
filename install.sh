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
#
# Every step lives in a function and the only top-level command is the final `{ main "$@"; }`: if `curl | sh`
# gets a truncated file, the shell defines functions at most (or stops at a syntax error) and runs nothing.
# The braces matter: a cut right after `main` would otherwise run main without its arguments.
set -eu

say() { printf '%s\n' "$*"; }
die() { printf 'basekit: %s\n' "$*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

usage() {
  cat <<'EOF'
Instala basekit desde un release de GitHub.

Uso: install.sh [--version vX.Y.Z] [--dir <directorio>]

  --version  versión a instalar (por defecto la última; también BASEKIT_VERSION)
  --dir      dónde instalar el binario (por defecto $HOME/.local/bin; también BASEKIT_INSTALL_DIR)
EOF
}

# parse_args sets version and dir; flags win over BASEKIT_VERSION and BASEKIT_INSTALL_DIR.
parse_args() {
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
}

# detect_platform sets os, arch, bin and ext; release assets are named after GOOS/GOARCH.
detect_platform() {
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
  bin='basekit' ext='tar.gz'
  if [ "$os" = windows ]; then bin='basekit.exe' ext='zip'; fi
}

# last_line prints the last non-blank line of $1: the cause reported by a failed curl or wget.
last_line() { printf '%s\n' "$1" | sed '/^[[:space:]]*$/d' | tail -n 1; }

# gnu_wget succeeds for GNU wget; BusyBox wget (Alpine) has no --version, --tries, --timeout or --max-redirect.
gnu_wget() {
  case "$(wget --version 2>/dev/null || true)" in
    *"GNU Wget"*) return 0 ;;
    *) return 1 ;;
  esac
}

# wget_get <url> <file>: GNU wget retries by itself; BusyBox wget gets a loop that skips HTTP 4xx answers.
# Leaves wget's output in err.
wget_get() {
  if gnu_wget; then
    err=$(wget -nv --timeout="$connect_timeout" --tries="$((retries + 1))" -O "$2" "$1" 2>&1) && return 0
    return 1
  fi
  attempt=0
  while :; do
    err=$(wget -q -T "$connect_timeout" -O "$2" "$1" 2>&1) && return 0
    case "$err" in *"server returned error: HTTP/"*" 4"[0-9][0-9]" "*) return 1 ;; esac
    attempt=$((attempt + 1))
    [ "$attempt" -le "$retries" ] || return 1
    sleep "$attempt"
  done
}

# curl_supports_retry_all succeeds when curl knows --retry-all-errors (7.71+); older curl rejects unknown options.
# Newer curl lists it only under `--help all`, older curl prints every option with `--help`.
curl_supports_retry_all() {
  curl --help all 2>/dev/null | grep -q -- --retry-all-errors ||
    curl --help 2>/dev/null | grep -q -- --retry-all-errors
}

# curl_retry runs curl, adding --retry-all-errors when supported so that --retry also covers refused connections
# and transfers reset midway (plain --retry skips them).
curl_retry() {
  if [ "$curl_retry_all" = 1 ]; then curl --retry-all-errors "$@"; else curl "$@"; fi
}

# fetch <url> <file>: downloads with curl, else wget, with timeouts and bounded retries; on failure it dies
# showing the URL and the downloader's error.
fetch() {
  err=""
  if have curl; then
    if err=$(curl_retry -fsSL --show-error --connect-timeout "$connect_timeout" --max-time "$max_time" --retry "$retries" \
      -o "$2" "$1" 2>&1); then return 0; fi
  elif have wget; then
    if wget_get "$1" "$2"; then return 0; fi
  else
    die "se necesita curl o wget para descargar"
  fi
  cause=$(last_line "$err")
  die "no se pudo descargar $1${cause:+: $cause}"
}

# wget_headers <url>: prints the headers of every response while following redirects, without downloading.
# Works with GNU and BusyBox wget (BusyBox lacks --max-redirect, so the redirect cannot be stopped).
wget_headers() {
  if gnu_wget; then wget -S --spider --timeout="$connect_timeout" --tries="$((retries + 1))" "$1" 2>&1
  else wget -S --spider -T "$connect_timeout" "$1" 2>&1
  fi
}

# latest_tag: follows the releases/latest redirect (no GitHub API, so no rate limit).
latest_tag() {
  url="https://github.com/$repo/releases/latest"
  cause=""
  if have curl; then
    # stdout and stderr together: the last line that is not a curl error is the final URL, the last
    # "curl: (N) …" line is the cause of a failure (DNS, TLS, proxy…).
    if out=$(curl_retry -fsSLI --show-error --connect-timeout "$connect_timeout" --max-time 60 --retry "$retries" \
      -o /dev/null -w '\n%{url_effective}' "$url" 2>&1); then
      final=$(last_line "$(printf '%s\n' "$out" | sed '/^curl: /d')")
    else
      final=""
    fi
    cause=$(printf '%s\n' "$out" | sed -n '/^curl: /p' | tail -n 1)
  elif have wget; then
    # The last Location header is the tag page; GNU wget also logs "Location: <url> [following]".
    final=$(wget_headers "$url" | sed -n 's/^ *[Ll]ocation: *//p' | tr -d '\r' | sed 's/ \[following\]$//' | tail -n 1) || final=""
  else
    die "se necesita curl o wget para descargar"
  fi
  case "$final" in
    */releases/tag/*) printf '%s\n' "${final##*/}" ;;
    *) die "no se pudo averiguar la última versión${cause:+ ($cause)}; indícala con --version vX.Y.Z" ;;
  esac
}

# resolve_release sets ver, tag, asset and base from version (resolving the latest release when empty).
resolve_release() {
  if [ -z "$version" ]; then version=$(latest_tag); fi
  ver=${version#v}
  tag="v$ver"
  asset="basekit_${ver}_${os}_${arch}.${ext}"
  base="${BASEKIT_DOWNLOAD_URL:-https://github.com/$repo/releases/download/$tag}"
  base=${base%/}
}

# resolve_dir turns dir into an absolute path without a trailing slash (default $HOME/.local/bin).
resolve_dir() {
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
}

cleanup() {
  if [ -n "$tmp" ]; then rm -rf "$tmp"; fi
  if [ -n "$staged" ]; then rm -f "$staged"; fi
}

# download_and_verify fetches the asset and checksums.txt into tmp and checks the sha256 before any extraction.
download_and_verify() {
  say "Descargando basekit $tag ($os/$arch)..."
  fetch "$base/$asset" "$tmp/$asset"
  fetch "$base/checksums.txt" "$tmp/checksums.txt"

  # A checksums.txt saved with CRLF line endings still matches: the trailing \r is dropped first.
  want=$(awk -v f="$asset" '{ sub(/\r$/, "") } $2 == f || $2 == "*" f { print tolower($1); exit }' "$tmp/checksums.txt")
  [ -n "$want" ] || die "checksums.txt no tiene $asset; no se instaló nada"
  if have sha256sum; then got=$(sha256sum "$tmp/$asset" | awk '{ print $1 }')
  elif have shasum; then got=$(shasum -a 256 "$tmp/$asset" | awk '{ print $1 }')
  else die "se necesita sha256sum o shasum para verificar la descarga"
  fi
  [ "$got" = "$want" ] || die "el checksum no coincide para $asset (esperado $want, obtenido $got); no se instaló nada"
}

# extract unpacks the verified asset into tmp/x and sets src to the binary inside it.
extract() {
  if [ "$ext" = zip ]; then
    have unzip || die "se necesita unzip para extraer $asset"
    unzip -q "$tmp/$asset" -d "$tmp/x" || die "no se pudo extraer $asset"
  else
    mkdir "$tmp/x"
    tar -xzf "$tmp/$asset" -C "$tmp/x" || die "no se pudo extraer $asset"
  fi
  src="$tmp/x/basekit_${ver}_${os}_${arch}/$bin"
  [ -f "$src" ] || die "$asset no contiene $bin"
}

# install_binary stages the binary next to its target and renames it, so a failure never leaves half a file.
install_binary() {
  mkdir -p "$dir" || die "no se pudo crear $dir"
  staged="$dir/.$bin.tmp.$$"
  cp "$src" "$staged" || die "no se pudo escribir en $dir"
  chmod 755 "$staged"
  mv -f "$staged" "$dir/$bin" || die "no se pudo reemplazar $dir/$bin"
  staged=""
}

# report runs the installed binary and prints the PATH line to add when dir is not in PATH.
report() {
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
}

main() {
  repo="F4R1DS4NCH3Z26/basekit-go"
  # Download limits: seconds to connect, seconds for a whole download, extra attempts after a transient failure.
  connect_timeout=15
  max_time=300
  retries=2
  curl_retry_all=0
  if have curl && curl_supports_retry_all; then curl_retry_all=1; fi

  parse_args "$@"
  detect_platform
  resolve_release
  resolve_dir

  # Work in a temp dir removed on exit, also on failure.
  tmp=""
  staged=""
  trap cleanup EXIT
  trap 'exit 1' HUP INT TERM
  tmp=$(mktemp -d 2>/dev/null || mktemp -d -t basekit) || die "no se pudo crear un directorio temporal"

  download_and_verify
  extract
  install_binary
  report
}

{ main "$@"; }
