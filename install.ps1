# Installs the basekit binary from a GitHub release on Windows (Windows PowerShell 5.1+ and PowerShell 7+).
#
#   irm https://raw.githubusercontent.com/F4R1DS4NCH3Z26/basekit-go/main/install.ps1 | iex
#   & ([scriptblock]::Create((irm https://raw.githubusercontent.com/F4R1DS4NCH3Z26/basekit-go/main/install.ps1))) -Version v0.1.0 -Dir C:\tools\basekit
#
# Environment:
#   BASEKIT_VERSION       same as -Version (default: the latest release)
#   BASEKIT_INSTALL_DIR   same as -Dir (default: $env:LOCALAPPDATA\Programs\basekit)
#   BASEKIT_DOWNLOAD_URL  directory holding the release assets (default:
#                         https://github.com/F4R1DS4NCH3Z26/basekit-go/releases/download/<tag>); file:/// works
#   BASEKIT_OS, BASEKIT_ARCH  test-only overrides of [Environment]::OSVersion.Platform and PROCESSOR_ARCHITECTURE
#
# The file is kept ASCII-only: Windows PowerShell 5.1 reads BOM-less scripts with the ANSI code page, so the
# Spanish accents in messages are written as [char] codes.
param(
  [string]$Version = $env:BASEKIT_VERSION,
  [string]$Dir = $env:BASEKIT_INSTALL_DIR
)

# Under `irm | iex` an `exit` would close the user's terminal: only a script run as a file exits with a code.
$basekitAsFile = $MyInvocation.MyCommand.CommandType -eq 'ExternalScript'

function Install-Basekit {
  param([string]$Version, [string]$Dir)
  # Set here, not at the top: under `irm | iex` top-level variables would leak into the user's session.
  # Preference variables are dynamically scoped, so the helper functions called from here inherit them.
  $ErrorActionPreference = 'Stop'
  $ProgressPreference = 'SilentlyContinue'

  $repo = 'F4R1DS4NCH3Z26/basekit-go'
  $a = [char]0xE1; $e = [char]0xE9; $i = [char]0xED; $o = [char]0xF3

  $osName = if ($env:BASEKIT_OS) { $env:BASEKIT_OS } else { [Environment]::OSVersion.Platform.ToString() }
  if ($osName -ne 'Win32NT') {
    throw "sistema operativo no soportado: $osName (install.ps1 es para Windows; en macOS, Linux o WSL usa install.sh)"
  }
  $archRaw = $env:BASEKIT_ARCH
  if (-not $archRaw) { $archRaw = $env:PROCESSOR_ARCHITEW6432 }
  if (-not $archRaw) { $archRaw = $env:PROCESSOR_ARCHITECTURE }
  switch ($archRaw) {
    'AMD64' { $arch = 'amd64' }
    'ARM64' { $arch = 'arm64' }
    default { throw "arquitectura no soportada: $archRaw (soportadas: AMD64 y ARM64)" }
  }

  # GitHub requires TLS 1.2, which Windows PowerShell 5.1 may not enable by default.
  try {
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
  } catch { }

  if (-not $Version) {
    # Follow the releases/latest redirect (no GitHub API, so no rate limit).
    $final = ''
    $cause = ''
    try {
      $req = [Net.WebRequest]::Create("https://github.com/$repo/releases/latest")
      $req.Method = 'HEAD'
      $req.Timeout = 60000
      $resp = $req.GetResponse()
      $final = $resp.ResponseUri.AbsoluteUri
      $resp.Close()
    } catch { $cause = " ($($_.Exception.Message))" }
    if ($final -notmatch '/releases/tag/([^/]+)$') {
      throw "no se pudo averiguar la $([char]0xFA)ltima versi${o}n$cause; ind${i}cala con -Version vX.Y.Z"
    }
    $Version = $Matches[1]
  }
  $ver = $Version -replace '^v', ''
  $tag = "v$ver"
  $asset = "basekit_${ver}_windows_$arch.zip"
  $base = if ($env:BASEKIT_DOWNLOAD_URL) { $env:BASEKIT_DOWNLOAD_URL } else { "https://github.com/$repo/releases/download/$tag" }
  $base = $base.TrimEnd('/')

  if (-not $Dir) { $Dir = Join-Path $env:LOCALAPPDATA 'Programs\basekit' }
  $Dir = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($Dir).TrimEnd('\')
  $target = Join-Path $Dir 'basekit.exe'

  $tmp = Join-Path ([IO.Path]::GetTempPath()) ('basekit-' + [Guid]::NewGuid().ToString('N'))
  $staged = Join-Path $Dir ".basekit.exe.tmp.$PID"
  New-Item -ItemType Directory -Path $tmp | Out-Null
  try {
    Write-Host "Descargando basekit $tag (windows/$arch)..."
    $zip = Join-Path $tmp $asset
    $sums = Join-Path $tmp 'checksums.txt'
    Get-ReleaseFile "$base/$asset" $zip
    Get-ReleaseFile "$base/checksums.txt" $sums

    $want = $null
    foreach ($line in Get-Content -LiteralPath $sums) {
      $parts = $line.Trim() -split '\s+'
      if ($parts.Count -ge 2 -and ($parts[1] -eq $asset -or $parts[1] -eq "*$asset")) { $want = $parts[0]; break }
    }
    if (-not $want) { throw "checksums.txt no tiene $asset; no se instal$o nada" }
    $got = (Get-FileHash -Algorithm SHA256 -LiteralPath $zip).Hash.ToLowerInvariant()
    if ($got -ne $want.ToLowerInvariant()) {
      throw "el checksum no coincide para $asset (esperado $want, obtenido $got); no se instal$o nada"
    }

    $x = Join-Path $tmp 'x'
    try { Expand-Archive -LiteralPath $zip -DestinationPath $x -Force } catch { throw "no se pudo extraer $asset" }
    $src = Join-Path $x "basekit_${ver}_windows_$arch\basekit.exe"
    if (-not (Test-Path -LiteralPath $src -PathType Leaf)) { throw "$asset no contiene basekit.exe" }

    # Stage next to the target, then swap it in, so a failure never leaves a half-written basekit.exe.
    try {
      New-Item -ItemType Directory -Force -Path $Dir | Out-Null
      [IO.File]::Copy($src, $staged, $true)
      # [NullString]::Value: PowerShell turns $null into an empty string for string parameters, and File.Replace rejects "".
      if (Test-Path -LiteralPath $target) { [IO.File]::Replace($staged, $target, [NullString]::Value) } else { [IO.File]::Move($staged, $target) }
    } catch {
      throw "no se pudo escribir $target ($($_.Exception.Message)); si basekit est$a abierto, ci${e}rralo y repite"
    }
  } finally {
    Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath $staged) { Remove-Item -LiteralPath $staged -Force -ErrorAction SilentlyContinue }
  }

  $installed = (& $target --version | Out-String).Trim()
  if ($LASTEXITCODE -ne 0) { throw "basekit se instal$o en $target pero no se pudo ejecutar" }
  Write-Host "basekit $installed instalado en $target"

  if (Add-UserPath $Dir) {
    Write-Host ''
    Write-Host "Se agreg$o $Dir a tu PATH de usuario. Abre una terminal nueva para usar basekit."
  }
}

# Get-ReleaseFile downloads a URL to a file with a 300 s timeout and up to 3 attempts (an HTTP 4xx answer is
# not retried); file:/// URLs are copied (PowerShell 7 cannot fetch them). A failure names the URL and its cause.
function Get-ReleaseFile([string]$Url, [string]$OutFile) {
  for ($attempt = 1; ; $attempt++) {
    $isFile = $false
    try {
      $uri = [Uri]$Url
      $isFile = $uri.IsFile
      if ($isFile) { Copy-Item -LiteralPath $uri.LocalPath -Destination $OutFile }
      else { Invoke-WebRequest -Uri $Url -OutFile $OutFile -UseBasicParsing -TimeoutSec 300 }
      return
    } catch {
      $cause = $_.Exception.Message
      $status = 0
      try { $status = [int]$_.Exception.Response.StatusCode } catch { }
      if ($isFile -or ($status -ge 400 -and $status -lt 500) -or $attempt -ge 3) {
        throw "no se pudo descargar ${Url}: $cause"
      }
      Start-Sleep -Seconds $attempt
    }
  }
}

# Test-BasekitNonInteractive tells whether the host runs a command and then exits (-NonInteractive, or
# -Command / -EncodedCommand without -NoExit, as in `powershell -c "irm ... | iex"`), so an error must fail it.
function Test-BasekitNonInteractive {
  $nonInteractive = $false; $command = $false; $noExit = $false
  foreach ($arg in [Environment]::GetCommandLineArgs()) {
    if ($arg -notmatch '^[-/]([A-Za-z]+)$') { continue }
    $name = $Matches[1].ToLowerInvariant()
    if ($name.Length -ge 4 -and 'noninteractive'.StartsWith($name)) { $nonInteractive = $true }
    elseif ($name.Length -ge 3 -and 'noexit'.StartsWith($name)) { $noExit = $true }
    elseif ('command'.StartsWith($name) -or $name -eq 'e' -or $name -eq 'ec' -or ($name.Length -ge 2 -and 'encodedcommand'.StartsWith($name))) { $command = $true }
  }
  return ($nonInteractive -or ($command -and -not $noExit))
}

# Add-UserPath appends Dir to the user PATH in the registry (no admin) and returns $true when it was missing.
# It keeps the value's kind (REG_EXPAND_SZ), so entries like %USERPROFILE%\bin keep working.
function Add-UserPath([string]$Dir) {
  if (-not (($env:Path -split ';') -contains $Dir)) { $env:Path = "$Dir;$env:Path" }
  $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true)
  try {
    $current = [string]$key.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
    $entries = @($current -split ';' | Where-Object { $_ })
    foreach ($entry in $entries) {
      if ([Environment]::ExpandEnvironmentVariables($entry).TrimEnd('\') -eq $Dir) { return $false }
    }
    $kind = if ($current) { $key.GetValueKind('Path') } else { [Microsoft.Win32.RegistryValueKind]::ExpandString }
    $key.SetValue('Path', (($entries + $Dir) -join ';'), $kind)
  } finally {
    $key.Close()
  }
  # Tell running programs (Explorer, new terminals) that the environment changed; best effort.
  try {
    if (-not ('Basekit.NativeEnv' -as [type])) {
      Add-Type -Namespace Basekit -Name NativeEnv -MemberDefinition @'
[DllImport("user32.dll", SetLastError = true, CharSet = CharSet.Auto)]
public static extern IntPtr SendMessageTimeout(IntPtr hWnd, uint Msg, UIntPtr wParam, string lParam, uint fuFlags, uint uTimeout, out UIntPtr lpdwResult);
'@
    }
    $result = [UIntPtr]::Zero
    [void][Basekit.NativeEnv]::SendMessageTimeout([IntPtr]0xffff, 0x1A, [UIntPtr]::Zero, 'Environment', 2, 5000, [ref]$result)
  } catch { }
  return $true
}

# On failure: a script run as a file exits 1. Under `irm | iex` an `exit` would close the user's terminal, so
# an interactive session gets the message and $LASTEXITCODE = 1 and stays open, while a host that runs one
# command and exits gets a terminating error, so `powershell -c "irm ... | iex"` exits with code 1.
try {
  Install-Basekit -Version $Version -Dir $Dir
} catch {
  $basekitError = "basekit: $($_.Exception.Message)"
  if ($basekitAsFile) { Write-Host $basekitError -ForegroundColor Red; exit 1 }
  $global:LASTEXITCODE = 1
  if (Test-BasekitNonInteractive) { throw $basekitError }
  Write-Host $basekitError -ForegroundColor Red
}
