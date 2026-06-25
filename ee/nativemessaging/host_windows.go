//go:build windows

package nativemessaging

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/kolide/launcher/v2/ee/allowedcmd"
)

const (
	authenticodePathEnvVar = `LAUNCHER_NATIVEMESSAGING_VERIFY_PATH`
	// authenticodeVerifyCmd uses -LiteralPath over -FilePath, plus the authenticodePathEnvVar,
	// to avoid command injection in the case that the path to verify contains a single-quote escape
	// or wildcard glob characters.
	authenticodeVerifyCmd = `Get-AuthenticodeSignature -LiteralPath $env:` + authenticodePathEnvVar + ` | ` +
		`Where-Object {$_.Status -eq "Valid"} | ` +
		`ForEach-Object { $_.SignerCertificate.Subject }`
)

// allowlistedBrowsers maps allowlisted browsers to their expected publishers.
// In case of variable install locations, we allowlist the executable name rather than
// the full path.
var allowlistedBrowsers = map[string]string{
	"chrome.exe":  "Google LLC", // Covers stable, beta, dev, and canary
	"firefox.exe": "Mozilla Corporation",
}

// validateBrowser confirms that the given path is a known browser
// signed by a publisher in our allowlist.
func validateBrowser(ctx context.Context, browserPath string, browserProcessName string) error {
	publisher, found := allowlistedBrowsers[browserProcessName]
	if !found {
		return fmt.Errorf("name %s for browser process not in allowlisted browser names", browserProcessName)
	}

	// Run Get-AuthenticodeSignature to confirm the codesigning is valid,
	// and extract the subject to check against `publisher.`
	// See: https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.security/get-authenticodesignature
	authenticodeCmd, err := allowedcmd.Powershell.Cmd(ctx, "-Command", authenticodeVerifyCmd)
	if err != nil {
		return fmt.Errorf("creating powershell Get-AuthenticodeSignature cmd: %w", err)
	}
	authenticodeCmd.Env = append(os.Environ(), authenticodePathEnvVar+"="+browserPath)
	authenticodeOutput, err := authenticodeCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("running powershell Get-AuthenticodeSignature against %s: output: `%s`: %w", browserPath, string(authenticodeOutput), err)
	}
	gotSubject := string(authenticodeOutput)
	if !strings.Contains(gotSubject, fmt.Sprintf("O=%s", publisher)) {
		return fmt.Errorf("certificate not issued to expected publisher %s -- got %s", publisher, gotSubject)
	}

	return nil
}
