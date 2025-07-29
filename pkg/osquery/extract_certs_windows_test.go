//go:build windows
// +build windows

package osquery

import (
	"testing"

	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func TestExtractSystemCerts(t *testing.T) {
	t.Parallel()

	slogger := multislogger.NewNopLogger()

	// Try to extract system certificates
	certs, err := extractSystemCerts(slogger)

	// If successful, verify we got certificates
	require.NoError(t, err)
	require.NotNil(t, certs)
	require.Greater(t, len(certs), 0, "should extract at least one certificate")

	// Verify all returned certificates are CA certificates
	for i, cert := range certs {
		require.NotNil(t, cert, "certificate at index %d should not be nil", i)
		require.True(t, cert.IsCA, "certificate at index %d should be a CA certificate", i)
	}
}

func TestExtractCertsFromStore(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		storeName string
	}{
		{
			name:      "ROOT store",
			storeName: "ROOT",
		},
		{
			name:      "CA store",
			storeName: "CA",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			certs, err := extractCertsFromStore(tc.storeName)

			// If successful, verify certificates
			require.NoError(t, err)
			for _, cert := range certs {
				require.NotNil(t, cert)
				require.True(t, cert.IsCA, "only CA certificates should be extracted")
			}
		})
	}
}
