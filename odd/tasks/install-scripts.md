# Install scripts for the Go CLI

Objective: one-line install of the `basekit` binary from GitHub Releases on macOS, Linux, WSL, Git Bash and native Windows. Branch: `dev/farid` (the user merges PRs). Delivery: ask-on-risk. Test runner: `make verify` plus the install tests below.

## Specs

- **S1** `install.sh` at the repo root. User: "que implementar un install.sh para la instalcion de los CLI en cualquier SO, como podria hacerlo?" then "si" to the parent's plan (Go first).
  - POSIX `sh` (no bashisms; passes `shellcheck`), `set -eu`. Usage: `curl -fsSL https://raw.githubusercontent.com/F4R1DS4NCH3Z26/basekit-go/main/install.sh | sh`, and `sh -s -- --version v0.1.0 --dir ~/bin`.
  - Platform: `uname -s` → `darwin` / `linux` / `windows` (MINGW*, MSYS*, CYGWIN* → windows, `.zip` and `basekit.exe`, needs `unzip`); `uname -m` → `amd64` (x86_64, amd64) / `arm64` (arm64, aarch64). Anything else: a clear error naming the OS/arch and exit 1.
  - Version: `--version vX.Y.Z` or env `BASEKIT_VERSION`; default the latest release, resolved without the GitHub API (follow the `https://github.com/F4R1DS4NCH3Z26/basekit-go/releases/latest` redirect). Asset names drop the `v`: `basekit_<X.Y.Z>_<os>_<arch>.tar.gz|.zip`, holding `basekit_<X.Y.Z>_<os>_<arch>/basekit[.exe]` (see scripts/build-release.sh).
  - Download with `curl`, else `wget`; base URL `https://github.com/F4R1DS4NCH3Z26/basekit-go/releases/download/<tag>`, overridable with env `BASEKIT_DOWNLOAD_URL` (tests, mirrors; `file://` must work).
  - Verify the archive's sha256 against `checksums.txt` (`sha256sum`, else `shasum -a 256`) before extracting. Mismatch or missing entry: error, exit 1, nothing installed.
  - Install dir: `--dir <path>` or env `BASEKIT_INSTALL_DIR`; default `$HOME/.local/bin`; created if missing; no sudo. Atomic: extract in a temp dir (removed on exit, also on failure), then move into place. If the dir is not in `PATH`, print the line to add.
  - Finish by running the installed `basekit --version` and printing where it was installed. User-facing messages in Spanish, like the CLI.
- **S2** `install.ps1` at the repo root for Windows PowerShell 5.1+ and pwsh. Usage: `irm https://raw.githubusercontent.com/F4R1DS4NCH3Z26/basekit-go/main/install.ps1 | iex`. Same rules as S1: arch from `$env:PROCESSOR_ARCHITECTURE` (AMD64 → amd64, ARM64 → arm64), the `.zip` asset, `Get-FileHash` check against `checksums.txt`, params `-Version`/`-Dir` and the same env overrides, default dir `$env:LOCALAPPDATA\Programs\basekit`, adds the dir to the user `PATH` (user scope, no admin) and says to open a new terminal, ends with `basekit --version`.
- **S3** Tests without network: a fake release (built with scripts/build-release.sh or hand-made archives plus `checksums.txt`) served from a temp dir via `BASEKIT_DOWNLOAD_URL=file://…`. Cases: install succeeds and the binary runs; `--dir` and the env overrides; checksum mismatch → exit 1 and nothing installed; unsupported arch/OS → exit 1 with the message (testable through an override such as env `BASEKIT_OS`/`BASEKIT_ARCH` that only the tests use, documented as such); PATH hint printed when the dir is not in PATH. `shellcheck install.sh` runs in CI. CI runs the sh tests on ubuntu and macos and the PowerShell test on windows.
- **S4** Docs: README install section with both one-liners, the options, and the note that Node's basekit installs with npm (`basekit install`).

- **S5** Truncation-safe `curl | sh`: every step of install.sh lives in functions, and the only top-level statement that acts is a final `main "$@"` call, so a truncated download runs nothing (review R1-002/R4-curl-pipe-truncation). User: "sigue con las mejoras y luego sigue con node".
- **S6** BusyBox wget (Alpine without curl): resolving the latest version works with a wget that lacks `--max-redirect` (for example by reading the redirect `Location` header with `wget -S --spider`, or another method that works on both GNU and BusyBox wget); downloads work with both (review R3-sh-wget-latest-busybox).
- **S7** CRLF `checksums.txt` is accepted by install.sh (strip a trailing `\r` before comparing), like install.ps1 (review R3-sh-crlf-checksums).
- **S8** `irm … | iex` failures are visible to callers: after printing the error, the iex path sets a failing status the caller can see (for example `$global:LASTEXITCODE = 1` and a terminating `throw` only when not interactive, or another approach that keeps an interactive window open and still fails `powershell -Command "irm … | iex"`); document the chosen behavior (review R4-iex-failure-exit-zero).
- **S9** Downloads have a connect timeout, an overall timeout and bounded retries (curl `--connect-timeout`, `--max-time`, `--retry`; wget `--timeout`/`--tries`; `Invoke-WebRequest -TimeoutSec` with a small retry loop), and a failed download shows its cause (curl's error line, the exception message) next to the URL (reviews R4-no-timeout-or-retry, R4-download-error-cause-discarded).

## Tasks

| ID | Specs | Route | Status | Commit |
|---|---|---|---|---|
| T1 | S1–S4 | delegated writer (context backstop: long parent session) | done | 79d9793, 453ef6c |
| T2 | S5–S9 | delegated writer (context backstop) | done | this commit |

Risk: item 5 (installers and CI) → independent verifier after the writer.

## Log

- **L1** (2026-11-10) User: "que implementar un install.sh para la instalcion de los CLI en cualquier SO, como podria hacerlo? /skills". Parent proposed install.sh + install.ps1 for Go first (Go already publishes v0.1.0 binaries for darwin/linux/windows × amd64/arm64 with checksums.txt), then Node. User: "si".
- **L2** Verified: release v0.1.0 assets `basekit_0.1.0_{darwin,linux}_{amd64,arm64}.tar.gz`, `basekit_0.1.0_windows_{amd64,arm64}.zip`, `checksums.txt` in sha256sum format; `origin/feat/windows-releases` has no commits beyond main.
- **L3** T1 done: install.sh (POSIX, dash-clean), install.ps1 (PS 5.1 and 7, ASCII-only), scripts/test-install.sh (RED with 21 failures before install.sh, then OK) and scripts/test-install.ps1, CI jobs `install-sh` (ubuntu + macos, shellcheck on ubuntu) and `install-ps1` (windows), `make install-test`, README section. Parent: zone lines for install.sh/install.ps1, README updated for the now-public repo (no GOPRIVATE). Independent verifier: PASS; real install from the public v0.1.0 release printed `0.1.0`. Not run locally: install.ps1 and shellcheck (CI only). Advisories: CRLF checksums.txt rejected with a misleading message; `irm | iex` leaves script variables in the session; curl errors hidden.

- **L4** (2026-11-10) Review of 79d9793 asked one correction (R3-ps1-replace-null-backup, CRITICAL: `$null` became "" for File.Replace, so every Windows reinstall failed); fixed in 453ef6c with `[NullString]::Value`, validated, approved. PR #9. User: "sigue con las mejoras y luego sigue con node" → S5–S9 from the review advisories.
- **L5** T2 done: install.sh restructured into functions with a final `main "$@"`; wget path works for GNU and BusyBox (`wget -S --spider` Location headers); CRLF checksums accepted; curl `--connect-timeout 15 --max-time 300 --retry 2 --show-error` and the error cause printed; install.ps1 retries up to 3 times with `-TimeoutSec 300`, and under iex sets `$global:LASTEXITCODE = 1` and throws when the host is non-interactive (`-Command`/`-NonInteractive`). Also fixed shellcheck SC2209 (CI failure on PR #9). sh suite 65 checks OK (also under dash); RED had 16 failures. Not run locally: install.ps1 tests and shellcheck (CI only); real BusyBox/GNU wget only faked.
- **L6** (2026-11-10) User: "Si" to three review gaps. (1) last line `{ main "$@"; }` so a cut after `main` runs nothing; (2) `--retry-all-errors` when curl supports it (checked via `curl --help all`/`--help`; it also retries a 404, about 3 s more on a wrong `--version`); (3) latest_tag shows curl's cause. RED 4 failures, GREEN 82 checks under sh and dash; shellcheck clean. Same `{ main "$@"; }` fix in Node (basekit 518cc01).
- **L7** (2026-11-10) User: "sí, haz los 3 avisos en ambos repos". (1) Total caps: `--retry-max-time` (300 s downloads, 60 s latest lookup), BusyBox wget stops retrying after the 300 s window, install.ps1 stops retrying after 300 s (Stopwatch); limits named; README states the real worst case (~10 min with curl). (2) install.ps1 treats a bare command argument as non-interactive (new CI case `iex-positional`). (3) test case 6 expects `--retry-all-errors` only when the host curl supports it. sh suite 91 checks (sh and dash), RED 3 then GREEN; shellcheck clean; ps1 changes only run in CI.
