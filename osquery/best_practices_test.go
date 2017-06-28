package osquery

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBestPractices(t *testing.T) {
	instance, err := LaunchOsqueryInstance()
	require.NoError(t, err)

	results, err := instance.Query("select * from kolide_best_practices;")
	require.NoError(t, err)

	require.Len(t, results, 1)
	require.True(t, results[0]["password_required_from_screensaver"])
}
