//go:build darwin
// +build darwin

package allowedcmd

import (
	"testing"

	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func TestRunDisclaimed(t *testing.T) {
	t.Parallel()

	slogger := multislogger.New()
	err := RunDisclaimed(slogger, []string{"brew"})
	require.NoError(t, err)
}
