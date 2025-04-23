//go:build darwin
// +build darwin

package disclaim

import (
	"testing"

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
