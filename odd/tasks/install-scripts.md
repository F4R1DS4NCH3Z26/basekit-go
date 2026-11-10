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

## Tasks

| ID | Specs | Route | Status | Commit |
|---|---|---|---|---|
| T1 | S1–S4 | delegated writer (context backstop: long parent session) | done | this commit |

Risk: item 5 (installers and CI) → independent verifier after the writer.

## Log

- **L1** (2026-11-10) User: "que implementar un install.sh para la instalcion de los CLI en cualquier SO, como podria hacerlo? /skills". Parent proposed install.sh + install.ps1 for Go first (Go already publishes v0.1.0 binaries for darwin/linux/windows × amd64/arm64 with checksums.txt), then Node. User: "si".
- **L2** Verified: release v0.1.0 assets `basekit_0.1.0_{darwin,linux}_{amd64,arm64}.tar.gz`, `basekit_0.1.0_windows_{amd64,arm64}.zip`, `checksums.txt` in sha256sum format; `origin/feat/windows-releases` has no commits beyond main.
- **L3** T1 done: install.sh (POSIX, dash-clean), install.ps1 (PS 5.1 and 7, ASCII-only), scripts/test-install.sh (RED with 21 failures before install.sh, then OK) and scripts/test-install.ps1, CI jobs `install-sh` (ubuntu + macos, shellcheck on ubuntu) and `install-ps1` (windows), `make install-test`, README section. Parent: zone lines for install.sh/install.ps1, README updated for the now-public repo (no GOPRIVATE). Independent verifier: PASS; real install from the public v0.1.0 release printed `0.1.0`. Not run locally: install.ps1 and shellcheck (CI only). Advisories: CRLF checksums.txt rejected with a misleading message; `irm | iex` leaves script variables in the session; curl errors hidden.
