//go:build windows

package nativemessaging

import (
	"context"
	"fmt"
	"strings"

	"github.com/kolide/launcher/v2/ee/allowedcmd"
	"github.com/shirou/gopsutil/v4/process"
)

// allowlistedBrowsers maps allowlisted browsers to their expected publishers.
// In case of variable install locations, we allowlist the executable name rather than
// the full path.
var allowlistedBrowsers = map[string]string{
	`Google Chrome`:        "Google LLC",
	`Google Chrome Beta`:   "Google LLC",
	`Google Chrome Dev`:    "Google LLC",
	`Google Chrome Canary`: "Google LLC",
}

// validateBrowser confirms that the calling process is a known browser
// signed by a publisher in our allowlist.
func validateBrowser(ctx context.Context, proc *process.Process) error {
	browserProcessName, err := proc.NameWithContext(ctx)
	if err != nil {
		return fmt.Errorf("getting name for browser process: %w", err)
	}
	// Some older versions of Chrome launch via cmd.exe, so we have to go up
	// one more level.
	if browserProcessName == "cmd.exe" {
		ppid, err := proc.PpidWithContext(ctx)
		if err != nil {
			return fmt.Errorf("getting cmd.exe parent process: %w", err)
		}
		proc, err = process.NewProcessWithContext(ctx, ppid)
		if err != nil {
			return fmt.Errorf("getting cmd.exe parent process (%d): %w", ppid, err)
		}

		browserProcessName, err = proc.NameWithContext(ctx)
		if err != nil {
			return fmt.Errorf("getting name for browser process: %w", err)
		}
	}

	publisher, found := allowlistedBrowsers[browserProcessName]
	if !found {
		return fmt.Errorf("name %s for browser process not in allowlisted browser names", browserProcessName)
	}

	pathToVerify, err := proc.ExeWithContext(ctx)
	if err != nil {
		return fmt.Errorf("getting executable for browser process: %w", err)
	}

	// Run Get-AuthenticodeSignature to confirm the codesigning is valid,
	// and extract the subject to check against `publisher.`
	// See: https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.security/get-authenticodesignature
	cmdStr := fmt.Sprintf(`Get-AuthenticodeSignature -FilePath '%s' | Where-Object {$_.Status -eq "Valid"} | ForEach-Object { $_.SignerCertificate.Subject }`, pathToVerify)
	authenticodeCmd, err := allowedcmd.Powershell.Cmd(ctx, cmdStr)
	if err != nil {
		return fmt.Errorf("creating powershell Get-AuthenticodeSignature cmd: %w", err)
	}
	authenticodeOutput, err := authenticodeCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("running powershell Get-AuthenticodeSignature against %s: output: `%s`: %w", pathToVerify, string(authenticodeOutput), err)
	}
	gotSubject := string(authenticodeOutput)
	if !strings.Contains(gotSubject, fmt.Sprintf("O=%s", publisher)) {
		return fmt.Errorf("certificate not issued to expected publisher %s -- got %s", publisher, gotSubject)
	}

	return nil
}
