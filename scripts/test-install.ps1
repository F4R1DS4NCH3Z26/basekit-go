# Offline tests for install.ps1 (Windows): builds a fake windows release in a temp dir, serves it through
# BASEKIT_DOWNLOAD_URL=file:///… and checks exit code, output, installed binary and the user PATH.
# Runs every case under Windows PowerShell 5.1 (powershell.exe) and PowerShell 7 (pwsh) when present.
# The asset is built by hand with the same name, layout and ldflags as scripts/build-release.sh, because
# that script needs bash and zip, which Windows runners do not provide.
# Usage: pwsh -File scripts/test-install.ps1   (needs go; restores the user PATH when it finishes)
$ErrorActionPreference = 'Continue'
$ProgressPreference = 'SilentlyContinue'

$root = Split-Path -Parent $PSScriptRoot
$installer = Join-Path $root 'install.ps1'
$version = '0.0.0-test'
$work = Join-Path ([IO.Path]::GetTempPath()) ('basekit-test-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $work | Out-Null
$script:fails = 0

$archRaw = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
$arch = if ($archRaw -eq 'ARM64') { 'arm64' } else { 'amd64' }
$name = "basekit_${version}_windows_$arch"

Write-Host "== release falsa $version en $work"
$release = Join-Path $work 'release'
$stage = Join-Path $work "stage\$name"
New-Item -ItemType Directory -Path $release, $stage | Out-Null
$env:CGO_ENABLED = '0'; $env:GOOS = 'windows'; $env:GOARCH = $arch
Push-Location $root
& go build -trimpath -ldflags "-s -w -X github.com/F4R1DS4NCH3Z26/basekit-go/internal/scaffold.Version=$version" -o (Join-Path $stage 'basekit.exe') ./cmd/basekit
$buildRc = $LASTEXITCODE
Pop-Location
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED
if ($buildRc -ne 0) { Write-Host 'FAIL no se pudo compilar la release falsa'; exit 1 }
Copy-Item (Join-Path $root 'README.md') $stage
Compress-Archive -Path $stage -DestinationPath (Join-Path $release "$name.zip")

function Write-Checksums([string]$Dir, [string]$Text) {
  [IO.File]::WriteAllText((Join-Path $Dir 'checksums.txt'), $Text)
}
function Get-Url([string]$Dir) { 'file:///' + ($Dir -replace '\\', '/') }

$hash = (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $release "$name.zip")).Hash.ToLowerInvariant()
Write-Checksums $release "$hash  $name.zip`n"
$badSum = Join-Path $work 'bad-sum'
Copy-Item -Recurse $release $badSum
Write-Checksums $badSum (('0' * 64) + "  $name.zip`n")
$noEntry = Join-Path $work 'no-entry'
Copy-Item -Recurse $release $noEntry
Write-Checksums $noEntry (('0' * 64) + "  otro.zip`n")

$crlf = Join-Path $work 'crlf'
Copy-Item -Recurse $release $crlf
Write-Checksums $crlf "$hash  $name.zip`r`n"

# Run-Install: runs install.ps1 in a child shell with the given env vars (and a private TMP/TEMP), either as a
# file with -Arguments or piped into Invoke-Expression with -Iex (followed by the -After commands); -HostFlags
# are the child's flags (default -NonInteractive). Leaves $script:rc, $script:out and $script:caseDir.
function Run-Install {
  param([string]$Shell, [string]$Label, [hashtable]$Env = @{}, [string[]]$Arguments = @(), [switch]$Iex, [string]$After = '',
    [string[]]$HostFlags = @('-NonInteractive'))
  $script:label = "$Shell/$Label"
  $script:caseDir = Join-Path $work "$Shell-$Label"
  $tmp = Join-Path $script:caseDir 'tmp'
  New-Item -ItemType Directory -Force -Path $tmp | Out-Null
  $vars = @{ TMP = $tmp; TEMP = $tmp } + $Env
  $saved = @{}
  foreach ($k in $vars.Keys) { $saved[$k] = [Environment]::GetEnvironmentVariable($k); [Environment]::SetEnvironmentVariable($k, $vars[$k]) }
  try {
    if ($Iex) {
      $cmd = "Get-Content -Raw -LiteralPath '$installer' | Invoke-Expression; $After"
      $script:out = & $Shell -NoProfile @HostFlags -ExecutionPolicy Bypass -Command $cmd 2>&1 | Out-String
    } else {
      $script:out = & $Shell -NoProfile @HostFlags -ExecutionPolicy Bypass -File $installer @Arguments 2>&1 | Out-String
    }
    $script:rc = $LASTEXITCODE
  } finally {
    foreach ($k in $saved.Keys) { [Environment]::SetEnvironmentVariable($k, $saved[$k]) }
  }
}

function Pass([string]$What) { Write-Host "ok   ${script:label}: $What" }
function Fail([string]$What) {
  Write-Host "FAIL ${script:label}: $What"
  ($script:out -split "`n") | ForEach-Object { Write-Host "     | $_" }
  $script:fails++
}
function Expect-Rc([int]$Want) { if ($script:rc -eq $Want) { Pass "exit $Want" } else { Fail "exit $($script:rc), se esperaba $Want" } }
function Expect-Out([string]$Text) { if ($script:out.Contains($Text)) { Pass "dice '$Text'" } else { Fail "la salida no contiene '$Text'" } }
function Expect-NoOut([string]$Text) { if ($script:out.Contains($Text)) { Fail "la salida contiene '$Text'" } else { Pass "no dice '$Text'" } }
function Expect-Bin([string]$Path) {
  $got = if (Test-Path -LiteralPath $Path) { (& $Path --version | Out-String).Trim() } else { '' }
  if ($got -eq $version) { Pass "$Path instalado y --version = $version" } else { Fail "$Path no está instalado o --version dio '$got'" }
}
function Expect-NoBin([string]$Path) {
  if (Test-Path -LiteralPath $Path) { Fail "$Path no debería existir" } else { Pass "$Path no existe" }
  $parent = Split-Path -Parent $Path
  $left = if (Test-Path -LiteralPath $parent) { @(Get-ChildItem -Force -LiteralPath $parent -Filter '.basekit*') } else { @() }
  if ($left.Count -gt 0) { Fail "quedaron archivos temporales en el destino" } else { Pass 'sin archivos temporales en el destino' }
}
function Expect-TmpClean {
  $left = @(Get-ChildItem -Force -LiteralPath (Join-Path $script:caseDir 'tmp'))
  if ($left.Count -eq 0) { Pass 'TMP limpio' } else { Fail "quedó basura en TMP: $($left.Name -join ', ')" }
}
function Get-UserPath {
  $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment')
  try { [string]$key.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames) } finally { $key.Close() }
}
function Expect-UserPath([string]$Dir, [int]$Count) {
  $n = @((Get-UserPath) -split ';' | Where-Object { $_.TrimEnd('\') -eq $Dir }).Count
  if ($n -eq $Count) { Pass "PATH de usuario tiene $Dir $Count vez/veces" } else { Fail "PATH de usuario tiene $Dir $n veces, se esperaba $Count" }
}

$shells = @('powershell.exe', 'pwsh') | Where-Object { Get-Command $_ -ErrorAction SilentlyContinue }
if (-not $shells) { Write-Host 'FAIL no hay powershell.exe ni pwsh'; exit 1 }

# Snapshot the user PATH: the installer edits it on purpose and the tests put it back.
$envKey = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true)
$hadPath = $null -ne $envKey.GetValue('Path')
$oldPath = $envKey.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
$oldKind = if ($hadPath) { $envKey.GetValueKind('Path') } else { [Microsoft.Win32.RegistryValueKind]::ExpandString }
$envKey.Close()

$url = Get-Url $release
try {
  foreach ($sh in $shells) {
    # 1. -Version with a leading v and -Dir; replaces an existing binary; adds the dir to the user PATH once.
    $dir = Join-Path $work "$sh-flags\bin"
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    Set-Content -LiteralPath (Join-Path $dir 'basekit.exe') -Value 'old'
    Run-Install $sh flags @{ BASEKIT_DOWNLOAD_URL = $url } @('-Version', "v$version", '-Dir', $dir)
    Expect-Rc 0
    Expect-Bin (Join-Path $dir 'basekit.exe')
    Expect-Out "basekit $version instalado en $dir\basekit.exe"
    Expect-Out 'Abre una terminal nueva'
    Expect-UserPath $dir 1
    Expect-TmpClean
    Run-Install $sh flags-again @{ BASEKIT_DOWNLOAD_URL = $url } @('-Version', $version, '-Dir', $dir)
    Expect-Rc 0
    Expect-UserPath $dir 1

    # 2. Env overrides (BASEKIT_VERSION without v, BASEKIT_INSTALL_DIR), piped like `irm … | iex`.
    $dir = Join-Path $work "$sh-env\dest"
    Run-Install $sh env @{ BASEKIT_DOWNLOAD_URL = $url; BASEKIT_VERSION = $version; BASEKIT_INSTALL_DIR = $dir } -Iex
    Expect-Rc 0
    Expect-Bin (Join-Path $dir 'basekit.exe')
    Expect-Out "instalado en $dir\basekit.exe"
    Expect-TmpClean

    # 3. Default dir is $env:LOCALAPPDATA\Programs\basekit, created if missing.
    $local = Join-Path $work "$sh-default\local"
    Run-Install $sh default @{ BASEKIT_DOWNLOAD_URL = $url; LOCALAPPDATA = $local } @('-Version', $version)
    Expect-Rc 0
    Expect-Bin (Join-Path $local 'Programs\basekit\basekit.exe')
    Expect-TmpClean

    # 4. Checksum mismatch: exit 1, nothing installed.
    $dir = Join-Path $work "$sh-checksum\bin"
    Run-Install $sh checksum @{ BASEKIT_DOWNLOAD_URL = (Get-Url $badSum) } @('-Version', $version, '-Dir', $dir)
    Expect-Rc 1
    Expect-Out 'el checksum no coincide'
    Expect-NoBin (Join-Path $dir 'basekit.exe')
    Expect-TmpClean

    # 5. Asset missing from checksums.txt: exit 1, nothing installed.
    $dir = Join-Path $work "$sh-no-entry\bin"
    Run-Install $sh no-entry @{ BASEKIT_DOWNLOAD_URL = (Get-Url $noEntry) } @('-Version', $version, '-Dir', $dir)
    Expect-Rc 1
    Expect-Out 'checksums.txt no tiene'
    Expect-NoBin (Join-Path $dir 'basekit.exe')
    Expect-TmpClean

    # 6. Version not in the release: the download fails, exit 1, nothing installed.
    $dir = Join-Path $work "$sh-missing-version\bin"
    Run-Install $sh missing-version @{ BASEKIT_DOWNLOAD_URL = $url } @('-Version', 'v9.9.9', '-Dir', $dir)
    Expect-Rc 1
    Expect-Out 'no se pudo descargar'
    Expect-Out "basekit_9.9.9_windows_$arch.zip: "
    Expect-NoBin (Join-Path $dir 'basekit.exe')
    Expect-TmpClean

    # 6b. checksums.txt with CRLF line endings is accepted.
    $dir = Join-Path $work "$sh-crlf\bin"
    Run-Install $sh crlf @{ BASEKIT_DOWNLOAD_URL = (Get-Url $crlf) } @('-Version', $version, '-Dir', $dir)
    Expect-Rc 0
    Expect-Bin (Join-Path $dir 'basekit.exe')

    # 7. Unsupported OS (BASEKIT_OS is a test-only override of [Environment]::OSVersion.Platform).
    $dir = Join-Path $work "$sh-os\bin"
    Run-Install $sh os @{ BASEKIT_OS = 'Plan9'; BASEKIT_DOWNLOAD_URL = $url } @('-Version', $version, '-Dir', $dir)
    Expect-Rc 1
    Expect-Out 'sistema operativo no soportado: Plan9'
    Expect-NoBin (Join-Path $dir 'basekit.exe')

    # 8. Unsupported architecture (BASEKIT_ARCH is a test-only override of PROCESSOR_ARCHITECTURE).
    $dir = Join-Path $work "$sh-arch\bin"
    Run-Install $sh arch @{ BASEKIT_ARCH = 'MIPS'; BASEKIT_DOWNLOAD_URL = $url } @('-Version', $version, '-Dir', $dir)
    Expect-Rc 1
    Expect-Out 'arquitectura no soportada: MIPS'
    Expect-NoBin (Join-Path $dir 'basekit.exe')

    # 9. An error under `irm … | iex` in an interactive-style session (-NoExit, no -NonInteractive) reports it,
    #    leaves $LASTEXITCODE = 1 and does not close the session.
    Run-Install $sh iex-error @{ BASEKIT_ARCH = 'MIPS'; BASEKIT_VERSION = $version } -Iex -HostFlags @('-NoExit') `
      -After '''session viva''; ''rc='' + $global:LASTEXITCODE; exit 0'
    Expect-Out 'arquitectura no soportada: MIPS'
    Expect-Out 'session viva'
    Expect-Out 'rc=1'

    # 10. `powershell -Command "irm … | iex"` fails with exit code 1 (without and with -NonInteractive).
    Run-Install $sh iex-command @{ BASEKIT_ARCH = 'MIPS'; BASEKIT_VERSION = $version } -Iex -HostFlags @()
    Expect-Rc 1
    Expect-Out 'arquitectura no soportada: MIPS'
    Run-Install $sh iex-noninteractive @{ BASEKIT_ARCH = 'MIPS'; BASEKIT_VERSION = $version } -Iex -After "'no debe verse'"
    Expect-Rc 1
    Expect-Out 'arquitectura no soportada: MIPS'
    Expect-NoOut 'no debe verse'
  }
} finally {
  $envKey = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true)
  if ($hadPath) { $envKey.SetValue('Path', $oldPath, $oldKind) } else { $envKey.DeleteValue('Path', $false) }
  $envKey.Close()
  Remove-Item -Recurse -Force -LiteralPath $work -ErrorAction SilentlyContinue
}

if ($script:fails -ne 0) { Write-Host "test-install.ps1: $($script:fails) fallo(s)"; exit 1 }
Write-Host 'test-install.ps1: OK'
