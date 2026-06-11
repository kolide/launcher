//go:build darwin

package nativemessaging

import (
	"context"
	"fmt"
	"strings"

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
		return fmt.Errorf("name %s for browser process not in allowlisted chrome paths", browserProcessName)
	}

	pathToVerify, err := proc.ExeWithContext(ctx)
	if err != nil {
		return fmt.Errorf("getting executable for browser process: %w", err)
	}

	// Verify the codesigning for the app bundle
	if err := validateCodesigning(ctx, pathToVerify); err != nil {
		return fmt.Errorf("validating codesigning: %w", err)
	}

	// Validate that the codesigning is associated with an expected team identifier
	if err := validateTeamIdentifier(ctx, pathToVerify, teamIdentifier); err != nil {
		return fmt.Errorf("validating team identifier: %w", err)
	}

	return nil
}

// validateCodesigning runs codesign --verify against the given path to confirm
// whether it has a valid signature.
func validateCodesigning(ctx context.Context, pathToVerify string) error {
	verifyCmd, err := allowedcmd.Codesign.Cmd(ctx, "--verify", "--verbose", "--deep", pathToVerify)
	if err != nil {
		return fmt.Errorf("creating codesign --verify cmd: %w", err)
	}
	verifyOutput, err := verifyCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("running codesign --verify against %s: output: `%s`: %w", pathToVerify, string(verifyOutput), err)
	}

	return nil
}

// validateTeamIdentifier runs codesign --display against the given path, parses the output
// to extract the team identifier, and confirms it matches the expected identifier.
func validateTeamIdentifier(ctx context.Context, pathToVerify string, teamIdentifier string) error {
	displayCmd, err := allowedcmd.Codesign.Cmd(ctx, "--display", "--verbose", "--deep", pathToVerify)
	if err != nil {
		return fmt.Errorf("creating codesign --display cmd: %w", err)
	}
	displayOutput, err := displayCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("running codesign --display against %s: output: `%s`: %w", pathToVerify, string(displayOutput), err)
	}

	displayOutputStr := strings.ReplaceAll(string(displayOutput), "\r\n", "\n")
	for line := range strings.SplitSeq(displayOutputStr, "\n") {
		key, val, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		if key != "TeamIdentifier" {
			continue
		}
		foundTeamIdentifier := strings.TrimSpace(val)
		if foundTeamIdentifier != teamIdentifier {
			return fmt.Errorf("team identifier mismatch: expected %s, got %s", teamIdentifier, foundTeamIdentifier)
		}
		return nil
	}

	return fmt.Errorf("codesign --display output does not contain TeamIdentifier: %s", displayOutputStr)
}
