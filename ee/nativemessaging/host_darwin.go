//go:build darwin

package nativemessaging

import (
	"context"
	"fmt"

	"github.com/kolide/launcher/v2/ee/allowedcmd"
	"github.com/shirou/gopsutil/v4/process"
)

// allowlistedBrowsers maps allowlisted browsers to their expected team identifiers.
// In case of variable install locations, we allowlist the executable name rather than
// the full path.
// In testing, Google Chrome for Testing (installed via npx puppeteer browsers) and Chromium
// (installed via homebrew) were not codesigned adequately, and the Chromium one has a pending
// deprecation notice, so I omitted them from this list.
var allowlistedBrowsers = map[string]string{
	`Google Chrome`:        "EQHXZ8M8AV",
	`Google Chrome Beta`:   "EQHXZ8M8AV",
	`Google Chrome Dev`:    "EQHXZ8M8AV",
	`Google Chrome Canary`: "EQHXZ8M8AV",
}

// validateBrowser confirms that the calling process is a known browser on our allowlist
// that has a valid code signature belonging to a known team identifier.
func validateBrowser(ctx context.Context, proc *process.Process) error {
	// The calling process must be in our allowlist
	browserProcessName, err := proc.NameWithContext(ctx)
	if err != nil {
		return fmt.Errorf("getting name for browser process: %w", err)
	}
	teamIdentifier, found := allowlistedBrowsers[browserProcessName]
	if !found {
		return fmt.Errorf("name %s for browser process not in allowlisted browser names", browserProcessName)
	}

	pathToVerify, err := proc.ExeWithContext(ctx)
	if err != nil {
		return fmt.Errorf("getting executable for browser process: %w", err)
	}

	// Build our identity assertion:
	// 1. "anchor apple generic" guarantees the signing chain roots are in Apple's CA
	// 2. "certificate leaf[subject.OU] = <teamIdentifier>" guarantees the team ID matches what we expect
	identityAssertion := fmt.Sprintf(`anchor apple generic and certificate leaf[subject.OU] = "%s"`, teamIdentifier)
	verifyCmd, err := allowedcmd.Codesign.Cmd(ctx, "--verify", "--deep", "--verbose", "-R", "="+identityAssertion, pathToVerify)
	if err != nil {
		return fmt.Errorf("creating codesign --verify cmd: %w", err)
	}
	verifyOutput, err := verifyCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("running codesign --verify against %s: output: `%s`: %w", pathToVerify, string(verifyOutput), err)
	}

	return nil
}
