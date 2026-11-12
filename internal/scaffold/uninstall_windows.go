//go:build windows

package scaffold

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// userPathScript removes $env:BASEKIT_UNINSTALL_DIR from the user PATH in the registry, matching entries the way
// install.ps1's Add-UserPath does (environment variables expanded, trailing backslash ignored, case ignored) and
// keeping the value's kind (REG_EXPAND_SZ). The folder comes through the environment, never as script text.
const userPathScript = `$ErrorActionPreference = 'Stop'
$dir = $env:BASEKIT_UNINSTALL_DIR.TrimEnd('\')
$key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true)
if (-not $key) { exit 0 }
try {
  $current = [string]$key.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
  if (-not $current) { exit 0 }
  $entries = @($current -split ';' | Where-Object { $_ })
  $keep = @($entries | Where-Object { [Environment]::ExpandEnvironmentVariables($_).TrimEnd('\') -ne $dir })
  if ($keep.Count -eq $entries.Count) { exit 0 }
  $key.SetValue('Path', ($keep -join ';'), $key.GetValueKind('Path'))
} finally {
  $key.Close()
}
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
`

// removeFromUserPath removes dir from the user PATH (HKCU\Environment), the entry install.ps1 added.
func removeFromUserPath(dir string, log func(string)) error {
	if !runCmd(log, os.TempDir(), []string{"BASEKIT_UNINSTALL_DIR=" + dir}, "powershell", "-NoProfile", "-NonInteractive", "-Command", userPathScript) {
		return errors.New("no se pudo quitar " + dir + " del PATH de usuario")
	}
	return nil
}

// deleteAfterExit starts a detached cmd that waits about two seconds, so this process has exited, and then runs
// command from the folder cwd. /v:off keeps '!' literal; uninstall refuses paths with '%' before getting here.
func deleteAfterExit(cwd, command string) error {
	shell := os.Getenv("ComSpec")
	if shell == "" {
		shell = "cmd.exe"
	}
	cmd := exec.Command(shell)
	// /s keeps the quoted command line as is once its outer quotes are stripped.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine:       `cmd /d /v:off /s /c "ping -n 3 127.0.0.1 >nul & ` + command + `"`,
		HideWindow:    true,
		CreationFlags: 0x00000008 | syscall.CREATE_NEW_PROCESS_GROUP, // DETACHED_PROCESS
	}
	cmd.Dir = cwd
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
