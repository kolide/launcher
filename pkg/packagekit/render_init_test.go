package packagekit

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderInitEmpty(t *testing.T) {
	t.Parallel()

	expectedOutputStrings := []string{
		`NAME="empty"`,
	}

	var output bytes.Buffer
	err := RenderInit(context.TODO(), &output, emptyInitOptions())
	require.NoError(t, err)
	require.True(t, len(output.String()) > 100)

	for _, s := range expectedOutputStrings {
		require.Contains(t, output.String(), s)
	}

}

func TestRenderInitComplex(t *testing.T) {
	t.Parallel()

	expectedOutputStrings := []string{
		`NAME="kolide-app"`,
		`KOLIDE_LAUNCHER_OSQUERYD_PATH=/usr/local/kolide-app/bin/osqueryd`,
		`export KOLIDE_LAUNCHER_OSQUERYD_PATH`,
		`--with_initial_runner`,
	}

	var output bytes.Buffer
	err := RenderInit(context.TODO(), &output, complexInitOptions())
	require.NoError(t, err)
	require.True(t, len(output.String()) > 300)

	for _, s := range expectedOutputStrings {
		require.Contains(t, output.String(), s)
	}

}
