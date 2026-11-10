#!/bin/sh
# Offline tests for install.sh: builds a fake release with scripts/build-release.sh in a temp dir,
# serves it through BASEKIT_DOWNLOAD_URL=file://… and checks exit code, output and installed binary.
# Usage: sh scripts/test-install.sh   (needs go, curl, tar and zip; never touches the network)
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
version=0.0.0-test
work=$(mktemp -d 2>/dev/null || mktemp -d -t basekit-test)
trap 'rm -rf "$work"' EXIT
trap 'exit 1' INT TERM
fails=0

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
label=flags
mkdir -p "$work/case-flags/bin" && echo old > "$work/case-flags/bin/basekit"
run_install flags BASEKIT_DOWNLOAD_URL="file://$release" sh "$root/install.sh" --version "v$version" --dir "$work/case-flags/bin"
expect_rc 0
expect_bin "$work/case-flags/bin/basekit"
expect_out "basekit $version instalado en $work/case-flags/bin/basekit"
expect_out "export PATH=\"$work/case-flags/bin:\$PATH\""
expect_tmp_clean

# 2. Env overrides (BASEKIT_VERSION without v, BASEKIT_INSTALL_DIR), piped like `curl … | sh -s`.
label=env
run_install env BASEKIT_DOWNLOAD_URL="file://$release" BASEKIT_VERSION="$version" \
  BASEKIT_INSTALL_DIR="$work/case-env/dest" sh -c "sh -s < '$root/install.sh'"
expect_rc 0
expect_bin "$work/case-env/dest/basekit"
expect_out "instalado en $work/case-env/dest/basekit"
expect_tmp_clean

# 3. Default dir is $HOME/.local/bin, created if missing; already in PATH, so no hint.
label=default-dir
run_install default-dir BASEKIT_DOWNLOAD_URL="file://$release" \
  PATH="$work/case-default-dir/home/.local/bin:/usr/bin:/bin:/usr/sbin:/sbin" sh "$root/install.sh" --version "$version"
expect_rc 0
expect_bin "$work/case-default-dir/home/.local/bin/basekit"
expect_no_out "export PATH="
expect_tmp_clean

# 3b. A literal "~/" in --dir=… (the shell does not expand it there) means $HOME.
label=tilde
run_install tilde BASEKIT_DOWNLOAD_URL="file://$release" sh "$root/install.sh" --version "$version" "--dir=~/mybin"
expect_rc 0
expect_bin "$work/case-tilde/home/mybin/basekit"

# 4. Checksum mismatch: exit 1, nothing installed.
label=checksum
cp -R "$release" "$work/bad-sum"
sed 's/^[0-9a-f]\{64\}/0000000000000000000000000000000000000000000000000000000000000000/' \
  "$release/checksums.txt" > "$work/bad-sum/checksums.txt"
run_install checksum BASEKIT_DOWNLOAD_URL="file://$work/bad-sum" sh "$root/install.sh" --version "$version" --dir "$work/case-checksum/bin"
expect_rc 1
expect_out "el checksum no coincide"
expect_no_bin "$work/case-checksum/bin/basekit"
expect_tmp_clean

# 5. Asset missing from checksums.txt: exit 1, nothing installed.
label=no-entry
cp -R "$release" "$work/no-entry"
echo "0000000000000000000000000000000000000000000000000000000000000000  otro.tar.gz" > "$work/no-entry/checksums.txt"
run_install no-entry BASEKIT_DOWNLOAD_URL="file://$work/no-entry" sh "$root/install.sh" --version "$version" --dir "$work/case-no-entry/bin"
expect_rc 1
expect_out "checksums.txt no tiene"
expect_no_bin "$work/case-no-entry/bin/basekit"
expect_tmp_clean

# 6. Version not in the release: the download fails, exit 1, nothing installed.
label=missing-version
run_install missing-version BASEKIT_DOWNLOAD_URL="file://$release" sh "$root/install.sh" --version v9.9.9 --dir "$work/case-missing-version/bin"
expect_rc 1
expect_out "no se pudo descargar"
expect_no_bin "$work/case-missing-version/bin/basekit"
expect_tmp_clean

# 7. Unsupported OS (BASEKIT_OS is a test-only override of `uname -s`).
label=os
run_install os BASEKIT_OS=Plan9 BASEKIT_DOWNLOAD_URL="file://$release" sh "$root/install.sh" --version "$version" --dir "$work/case-os/bin"
expect_rc 1
expect_out "sistema operativo no soportado: Plan9"
expect_no_bin "$work/case-os/bin/basekit"

# 8. Unsupported architecture (BASEKIT_ARCH is a test-only override of `uname -m`).
label=arch
run_install arch BASEKIT_ARCH=mips BASEKIT_DOWNLOAD_URL="file://$release" sh "$root/install.sh" --version "$version" --dir "$work/case-arch/bin"
expect_rc 1
expect_out "arquitectura no soportada: mips"
expect_no_bin "$work/case-arch/bin/basekit"

# 9. Unknown option: exit 1 with the usage hint.
label=bad-flag
run_install bad-flag sh "$root/install.sh" --nope
expect_rc 1
expect_out "opción desconocida: --nope"

if [ "$fails" -ne 0 ]; then echo "test-install: $fails fallo(s)"; exit 1; fi
echo "test-install: OK"
