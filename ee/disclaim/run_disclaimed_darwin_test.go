//go:build darwin
// +build darwin

package disclaim

import (
	"testing"

	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func TestRunDisclaimedDoesNotRunArbitraryCommands(t *testing.T) {
	t.Parallel()

	slogger := multislogger.New()
	err := RunDisclaimed(slogger, []string{"reboot"})
	require.Error(t, err, "expected rundisclaimed to err for unknown command")
	require.Contains(t, err.Error(), "unsupported command")
}

func TestRunDisclaimedDoesNotRunArbitraryOptions(t *testing.T) {
	t.Parallel()

	slogger := multislogger.New()
	err := RunDisclaimed(slogger, []string{"brew", "outdated", "--json", "&&", "reboot"})
	require.Error(t, err, "expected rundisclaimed to err for unknown command options")
	require.Contains(t, err.Error(), "invalid argument provided")
}

// TestRunDisclaimedDoesNotErr tests only the happy path of running a basic command (echo)
// through our C bindings for spawn_disclaimed. This is meant as a basic sanity test that it works.
// It does not test that the disclaim itself works, but we are hoping to figure out a test for that
// in the future
func TestRunDisclaimedDoesNotErr(t *testing.T) { // nolint:paralleltest // writes to package level var allowedCmdGenerators
	allowedCmdGenerators["echo"] = allowedCmdGenerator{
		allowedOpts: map[string]struct{}{
			"hello": {},
		},
		generate: allowedcmd.Echo,
	}

	slogger := multislogger.New()
	err := RunDisclaimed(slogger, []string{"echo", "hello"})
	require.NoError(t, err)
}
