#!/bin/sh
# Offline tests for install.sh: builds a fake release with scripts/build-release.sh in a temp dir,
# serves it through BASEKIT_DOWNLOAD_URL=file://… and checks exit code, output and installed binary.
# Usage: sh scripts/test-install.sh   (needs go, curl, tar and zip; never touches the network)
#   INSTALL_SH=/bin/dash sh scripts/test-install.sh   runs install.sh with another shell (default: sh)
# Some checks depend on the host curl: --retry-all-errors (curl 7.71+) is expected in install.sh's curl calls
# only when the host curl lists it, probed the same way install.sh does; case 6b fakes an older curl.
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
version=0.0.0-test
work=$(mktemp -d 2>/dev/null || mktemp -d -t basekit-test)
trap 'rm -rf "$work"' EXIT
trap 'exit 1' INT TERM
fails=0
sh_cmd=${INSTALL_SH:-sh}
# Same probe as install.sh's curl_supports_retry_all.
host_retry_all=0
if curl --help all 2>/dev/null | grep -q -- --retry-all-errors ||
  curl --help 2>/dev/null | grep -q -- --retry-all-errors; then host_retry_all=1; fi

echo "== release falsa $version en $work"
# build-release.sh writes ./dist relative to the current directory: build from a copy, so the worktree stays clean.
mkdir -p "$work/src"
(cd "$root" && cp -R go.mod go.sum README.md cmd internal plantillas "$work/src/")
(cd "$work/src" && bash "$root/scripts/build-release.sh" "$version" >/dev/null)
release="$work/src/dist"

# run_install <label> [env assignments…] -- [install.sh args…]: runs install.sh with a clean HOME and TMPDIR,
# leaving the exit code in $rc and stdout+stderr in $out.
run_install() {
  label=$1; shift
  case_dir="$work/case-$label"
  mkdir -p "$case_dir/home" "$case_dir/tmp"
  set -- HOME="$case_dir/home" TMPDIR="$case_dir/tmp" PATH="/usr/bin:/bin:/usr/sbin:/sbin" "$@"
  rc=0
  out=$(env "$@" 2>&1) || rc=$?
}

pass() { echo "ok   $label: $1"; }
fail() { echo "FAIL $label: $1"; printf '%s\n' "$out" | sed 's/^/     | /'; fails=$((fails + 1)); }
expect_rc() { if [ "$rc" -eq "$1" ]; then pass "exit $1"; else fail "exit $rc, se esperaba $1"; fi; }
expect_out() { case "$out" in *"$1"*) pass "dice '$1'" ;; *) fail "la salida no contiene '$1'" ;; esac; }
expect_no_out() { case "$out" in *"$1"*) fail "la salida contiene '$1'" ;; *) pass "no dice '$1'" ;; esac; }
expect_bin() { # expect_bin <path>: present, executable and reporting the release version
  if [ -x "$1" ] && [ "$("$1" --version 2>&1)" = "$version" ]; then pass "$1 instalado y --version = $version"
  else fail "$1 no está instalado, no es ejecutable o --version no da $version"; fi
}
expect_no_bin() {
  if [ -e "$1" ]; then fail "$1 no debería existir"; else pass "$1 no existe"; fi
  leftovers=$(find "$(dirname "$1")" -name '.basekit*' 2>/dev/null || true)
  if [ -n "$leftovers" ]; then fail "quedaron archivos temporales: $leftovers"; else pass "sin archivos temporales en el destino"; fi
}
expect_tmp_clean() {
  junk=$(find "$case_dir/tmp" -mindepth 1 | head -n 5)
  if [ -z "$junk" ]; then pass "TMPDIR limpio"; else fail "quedó basura en TMPDIR: $junk"; fi
}

# 1. --version with a leading v and --dir; replaces an existing binary; prints the PATH hint.
label='flags'
mkdir -p "$work/case-flags/bin" && echo old > "$work/case-flags/bin/basekit"
run_install flags BASEKIT_DOWNLOAD_URL="file://$release" "$sh_cmd" "$root/install.sh" --version "v$version" --dir "$work/case-flags/bin"
expect_rc 0
expect_bin "$work/case-flags/bin/basekit"
expect_out "basekit $version instalado en $work/case-flags/bin/basekit"
expect_out "export PATH=\"$work/case-flags/bin:\$PATH\""
expect_tmp_clean

# 2. Env overrides (BASEKIT_VERSION without v, BASEKIT_INSTALL_DIR), piped like `curl … | sh -s`.
label='env'
run_install env BASEKIT_DOWNLOAD_URL="file://$release" BASEKIT_VERSION="$version" \
  BASEKIT_INSTALL_DIR="$work/case-env/dest" sh -c "'$sh_cmd' -s < '$root/install.sh'"
expect_rc 0
expect_bin "$work/case-env/dest/basekit"
expect_out "instalado en $work/case-env/dest/basekit"
expect_tmp_clean

# 3. Default dir is $HOME/.local/bin, created if missing; already in PATH, so no hint.
label='default-dir'
run_install default-dir BASEKIT_DOWNLOAD_URL="file://$release" \
  PATH="$work/case-default-dir/home/.local/bin:/usr/bin:/bin:/usr/sbin:/sbin" "$sh_cmd" "$root/install.sh" --version "$version"
expect_rc 0
expect_bin "$work/case-default-dir/home/.local/bin/basekit"
expect_no_out "export PATH="
expect_tmp_clean

# 3b. A literal "~/" in --dir=… (the shell does not expand it there) means $HOME.
label='tilde'
run_install tilde BASEKIT_DOWNLOAD_URL="file://$release" "$sh_cmd" "$root/install.sh" --version "$version" "--dir=~/mybin"
expect_rc 0
expect_bin "$work/case-tilde/home/mybin/basekit"

# 4. Checksum mismatch: exit 1, nothing installed.
label='checksum'
cp -R "$release" "$work/bad-sum"
sed 's/^[0-9a-f]\{64\}/0000000000000000000000000000000000000000000000000000000000000000/' \
  "$release/checksums.txt" > "$work/bad-sum/checksums.txt"
run_install checksum BASEKIT_DOWNLOAD_URL="file://$work/bad-sum" "$sh_cmd" "$root/install.sh" --version "$version" --dir "$work/case-checksum/bin"
expect_rc 1
expect_out "el checksum no coincide"
expect_no_bin "$work/case-checksum/bin/basekit"
expect_tmp_clean

# 5. Asset missing from checksums.txt: exit 1, nothing installed.
label='no-entry'
cp -R "$release" "$work/no-entry"
echo "0000000000000000000000000000000000000000000000000000000000000000  otro.tar.gz" > "$work/no-entry/checksums.txt"
run_install no-entry BASEKIT_DOWNLOAD_URL="file://$work/no-entry" "$sh_cmd" "$root/install.sh" --version "$version" --dir "$work/case-no-entry/bin"
expect_rc 1
expect_out "checksums.txt no tiene"
expect_no_bin "$work/case-no-entry/bin/basekit"
expect_tmp_clean

# 6. Version not in the release: the download fails with its cause next to the URL, exit 1, nothing installed.
#    A logging curl wrapper checks the download limits (connect timeout, time per attempt, retries and the
#    retry window that caps their total), and --retry-all-errors only when the host curl supports it.
label='missing-version'
mkdir -p "$work/curl-log"
sed -e "s|@LOG@|$work/curl-log/args|" -e "s|@CURL@|$(command -v curl)|" > "$work/curl-log/curl" <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >> "@LOG@"
exec "@CURL@" "$@"
EOF
chmod 755 "$work/curl-log/curl"
run_install missing-version BASEKIT_DOWNLOAD_URL="file://$release" PATH="$work/curl-log:/usr/bin:/bin:/usr/sbin:/sbin" \
  "$sh_cmd" "$root/install.sh" --version v9.9.9 --dir "$work/case-missing-version/bin"
expect_rc 1
expect_out "no se pudo descargar file://$release/basekit_9.9.9_"
expect_out "curl: (37)"
curl_args=$(cat "$work/curl-log/args" 2>/dev/null || true)
for flag in "--connect-timeout 15" "--max-time 300" "--retry 2" "--retry-max-time 300"; do
  case "$curl_args" in *"$flag "*) pass "curl recibe $flag" ;; *) fail "curl no recibe $flag: $curl_args" ;; esac
done
if [ "$host_retry_all" = 1 ]; then
  case "$curl_args" in *"--retry-all-errors "*) pass "curl recibe --retry-all-errors" ;; *) fail "curl no recibe --retry-all-errors: $curl_args" ;; esac
else
  case "$curl_args" in *--retry-all-errors*) fail "curl recibe --retry-all-errors sin soportarlo: $curl_args" ;; *) pass "curl no recibe --retry-all-errors (el curl del sistema no lo soporta)" ;; esac
fi
expect_no_bin "$work/case-missing-version/bin/basekit"
expect_tmp_clean

# 6b. An old curl (like 7.68 on Ubuntu 20.04) rejects --retry-all-errors: it is only passed when `curl --help` or
#     `curl --help all` lists it. The fake curl lists no such option and fails if it gets it.
label='old-curl'
mkdir -p "$work/old-curl"
sed -e "s|@LOG@|$work/old-curl/args|" -e "s|@CURL@|$(command -v curl)|" > "$work/old-curl/curl" <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >> "@LOG@"
for a in "$@"; do
  case "$a" in
    --help) printf '%s\n' "Usage: curl [options...] <url>" " -f, --fail  Fail silently" "     --retry <num>  Retry request"; exit 0 ;;
    --retry-all-errors) echo "curl: option --retry-all-errors: is unknown" >&2; exit 2 ;;
  esac
done
exec "@CURL@" "$@"
EOF
chmod 755 "$work/old-curl/curl"
run_install old-curl BASEKIT_DOWNLOAD_URL="file://$release" PATH="$work/old-curl:/usr/bin:/bin:/usr/sbin:/sbin" \
  "$sh_cmd" "$root/install.sh" --version "$version" --dir "$work/case-old-curl/bin"
expect_rc 0
expect_bin "$work/case-old-curl/bin/basekit"
curl_args=$(cat "$work/old-curl/args" 2>/dev/null || true)
case "$curl_args" in *--connect-timeout*) pass "se usó el curl falso" ;; *) fail "no se usó el curl falso: $curl_args" ;; esac
case "$curl_args" in *--retry-all-errors*) fail "curl recibe --retry-all-errors: $curl_args" ;; *) pass "curl no recibe --retry-all-errors" ;; esac
expect_tmp_clean

# 6c. When resolving the latest version fails, the error shows curl's cause (DNS, TLS, proxy...). The lookup
#     has its own limits: 60 s per attempt and a 60 s retry window.
label='latest-error'
mkdir -p "$work/dns-curl"
sed -e "s|@LOG@|$work/dns-curl/args|" -e "s|@CURL@|$(command -v curl)|" > "$work/dns-curl/curl" <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >> "@LOG@"
for a in "$@"; do
  case "$a" in
    https://github.com/*/releases/latest) echo "curl: (6) Could not resolve host: github.com" >&2; exit 6 ;;
  esac
done
exec "@CURL@" "$@"
EOF
chmod 755 "$work/dns-curl/curl"
run_install latest-error BASEKIT_DOWNLOAD_URL="file://$release" PATH="$work/dns-curl:/usr/bin:/bin:/usr/sbin:/sbin" \
  "$sh_cmd" "$root/install.sh" --dir "$work/case-latest-error/bin"
expect_rc 1
expect_out "no se pudo averiguar la última versión"
expect_out "curl: (6)"
expect_no_bin "$work/case-latest-error/bin/basekit"
curl_args=$(grep 'releases/latest' "$work/dns-curl/args" 2>/dev/null || true)
for flag in "--max-time 60" "--retry-max-time 60"; do
  case "$curl_args" in *"$flag "*) pass "curl recibe $flag" ;; *) fail "curl no recibe $flag: $curl_args" ;; esac
done

# 7. Unsupported OS (BASEKIT_OS is a test-only override of `uname -s`).
label='os'
run_install os BASEKIT_OS=Plan9 BASEKIT_DOWNLOAD_URL="file://$release" "$sh_cmd" "$root/install.sh" --version "$version" --dir "$work/case-os/bin"
expect_rc 1
expect_out "sistema operativo no soportado: Plan9"
expect_no_bin "$work/case-os/bin/basekit"

# 8. Unsupported architecture (BASEKIT_ARCH is a test-only override of `uname -m`).
label='arch'
run_install arch BASEKIT_ARCH=mips BASEKIT_DOWNLOAD_URL="file://$release" "$sh_cmd" "$root/install.sh" --version "$version" --dir "$work/case-arch/bin"
expect_rc 1
expect_out "arquitectura no soportada: mips"
expect_no_bin "$work/case-arch/bin/basekit"

# 9. Unknown option: exit 1 with the usage hint.
label='bad-flag'
run_install bad-flag "$sh_cmd" "$root/install.sh" --nope
expect_rc 1
expect_out "opción desconocida: --nope"

# 10. checksums.txt with CRLF line endings (written on Windows) is accepted.
label='crlf'
cp -R "$release" "$work/crlf"
awk '{ printf "%s\r\n", $0 }' "$release/checksums.txt" > "$work/crlf/checksums.txt"
run_install crlf BASEKIT_DOWNLOAD_URL="file://$work/crlf" "$sh_cmd" "$root/install.sh" --version "$version" --dir "$work/case-crlf/bin"
expect_rc 0
expect_bin "$work/case-crlf/bin/basekit"
expect_tmp_clean

# 11. A download cut short by `curl | sh` runs nothing: no install dir and a clean TMPDIR. Cut points: right
#     after the line that moves the binary into place, half the bytes, without the last line, inside it, and
#     right after `main` or `main ` on the last line (sh would run main without its arguments). The full script,
#     piped the same way, installs (positive control).
size=$(wc -c < "$root/install.sh")
lines=$(wc -l < "$root/install.sh")
mv_line=$(grep -n "mv -f \"\\\$staged\"" "$root/install.sh" | head -n 1 | cut -d: -f1)
[ -n "$mv_line" ] || { echo "FAIL truncated: install.sh no tiene la línea mv -f \"\$staged\""; exit 1; }
# Byte offset of the end of `main` on the last line (ASCII only; install.sh ends with a newline).
last=$(tail -n 1 "$root/install.sh")
main_col=$(printf '%s\n' "$last" | awk '{ print index($0, "main") }')
[ "$main_col" -gt 0 ] || { echo "FAIL truncated: la última línea de install.sh no llama a main: $last"; exit 1; }
after_main=$((size - $(printf '%s\n' "$last" | wc -c) + main_col - 1 + 4))
label='truncated-full'
run_install "$label" BASEKIT_DOWNLOAD_URL="file://$release" BASEKIT_VERSION="$version" \
  BASEKIT_INSTALL_DIR="$work/case-$label/bin" sh -c "'$sh_cmd' -s < '$root/install.sh'"
expect_rc 0
expect_bin "$work/case-$label/bin/basekit"
for cut in staged half no-last-line mid-last-line after-main after-main-space; do
  label=truncated-$cut
  part="$work/install-$cut.sh"
  case "$cut" in
    staged) head -n "$mv_line" "$root/install.sh" > "$part" ;;
    half) head -c "$((size / 2))" "$root/install.sh" > "$part" ;;
    no-last-line) head -n "$((lines - 1))" "$root/install.sh" > "$part" ;;
    mid-last-line) head -c "$((size - 4))" "$root/install.sh" > "$part" ;;
    after-main) head -c "$after_main" "$root/install.sh" > "$part" ;;
    after-main-space) head -c "$((after_main + 1))" "$root/install.sh" > "$part" ;;
  esac
  run_install "$label" BASEKIT_DOWNLOAD_URL="file://$release" BASEKIT_VERSION="$version" \
    BASEKIT_INSTALL_DIR="$work/case-$label/bin" sh -c "'$sh_cmd' -s < '$part'"
  if [ -e "$work/case-$label/bin" ]; then fail "se creó $work/case-$label/bin"; else pass "no se instaló nada"; fi
  expect_tmp_clean
done

# 12. Without curl, wget downloads and resolves the latest version. The fake wget serves file:// URLs through
#     curl and answers GitHub's releases/latest with a redirect to v$version. Like BusyBox wget (Alpine), the
#     busybox flavor rejects --max-redirect, --tries, --timeout, -nv and --version; the gnu flavor takes them.
tools="$work/tools"
mkdir -p "$tools"
for t in sh awk sed tr tail head cat mktemp rm mkdir cp mv chmod tar gzip uname sha256sum shasum perl sleep date; do
  for d in /usr/bin /bin /usr/sbin /sbin; do
    if [ -x "$d/$t" ]; then ln -s "$d/$t" "$tools/$t"; break; fi
  done
done
for flavor in busybox gnu; do
  label=wget-$flavor
  mkdir -p "$work/$label"
  sed -e "s|@FLAVOR@|$flavor|" -e "s|@CURL@|$(command -v curl)|" -e "s|@TAG@|v$version|" \
    -e "s|@LOG@|$work/$label/args|" > "$work/$label/wget" <<'EOF'
#!/bin/sh
flavor=@FLAVOR@
printf '%s\n' "$*" >> "@LOG@"
out=/dev/stdout headers=0 url=""
while [ $# -gt 0 ]; do
  case "$1" in
    --version) [ "$flavor" = gnu ] || { echo "wget: unrecognized option '--version'" >&2; exit 1; }; echo "GNU Wget 1.21.4"; exit 0 ;;
    --max-redirect* | --tries=* | --timeout=* | -nv) [ "$flavor" = gnu ] || { echo "wget: unrecognized option '$1'" >&2; exit 1; } ;;
    -q | --spider) ;;
    -S) headers=1 ;;
    -T) shift ;;
    -O) out=$2; shift ;;
    -*) echo "wget: unrecognized option '$1'" >&2; exit 1 ;;
    *) url=$1 ;;
  esac
  shift
done
case "$url" in
  https://github.com/*/releases/latest)
    if [ "$headers" = 1 ]; then
      printf '  HTTP/1.1 302 Found\r\n  Location: %s\r\n' "${url%/latest}/tag/@TAG@" >&2
      if [ "$flavor" = gnu ]; then printf 'Location: %s [following]\n' "${url%/latest}/tag/@TAG@" >&2; fi
      printf '  HTTP/1.1 200 OK\r\n' >&2
    fi
    ;;
  file://*) exec "@CURL@" -fsS -o "$out" "$url" ;;
  *) echo "wget: bad address '$url'" >&2; exit 1 ;;
esac
EOF
  chmod 755 "$work/$label/wget"
  run_install "$label" BASEKIT_DOWNLOAD_URL="file://$release" PATH="$work/$label:$tools" \
    "$sh_cmd" "$root/install.sh" --dir "$work/case-$label/bin"
  expect_rc 0
  expect_out "Descargando basekit v$version"
  expect_bin "$work/case-$label/bin/basekit"
  expect_tmp_clean
  wget_args=$(cat "$work/$label/args" 2>/dev/null || true)
  if [ "$flavor" = gnu ]; then want="--tries="; else want="-T "; fi
  case "$wget_args" in *"$want"*) pass "wget recibe $want" ;; *) fail "wget no recibe '$want': $wget_args" ;; esac
done

# 12b. BusyBox wget stops retrying once the 300 s retry window has passed since the first attempt. A fake
#      date jumps 1000 s per call, so a failing download (a missing version) gets a single attempt.
label='wget-busybox-window'
mkdir -p "$work/fake-date"
sed -e "s|@NOW@|$work/fake-date/now|" > "$work/fake-date/date" <<'EOF'
#!/bin/sh
n=$(cat "@NOW@" 2>/dev/null || echo 0)
echo "$((n + 1000))" > "@NOW@"
echo "$n"
EOF
chmod 755 "$work/fake-date/date"
: > "$work/wget-busybox/args"
run_install "$label" BASEKIT_DOWNLOAD_URL="file://$release" PATH="$work/fake-date:$work/wget-busybox:$tools" \
  "$sh_cmd" "$root/install.sh" --version v9.9.9 --dir "$work/case-$label/bin"
expect_rc 1
expect_out "no se pudo descargar file://$release/basekit_9.9.9_"
attempts=$(grep -c -- ' -O ' "$work/wget-busybox/args" || true)
if [ "$attempts" = 1 ]; then pass "un solo intento"; else fail "wget se llamó $attempts veces, se esperaba 1"; fi
expect_no_bin "$work/case-$label/bin/basekit"
expect_tmp_clean

if [ "$fails" -ne 0 ]; then echo "test-install: $fails fallo(s)"; exit 1; fi
echo "test-install: OK"
