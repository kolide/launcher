//go:build darwin
// +build darwin

package security

import (
	"bytes"
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	//go:embed test-data/admin_certs.txt
	adminCerts []byte

	//go:embed test-data/system_certs.txt
	systemCerts []byte

	//go:embed test-data/no_trust_settings.txt
	noTrustSettings []byte
)

func Test_parseTrustSettingsDump(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName                      string
		input                             []byte
		expectedCertCount                 int
		expectedTrustSettingsPerCertCount int
		expectedError                     bool
	}{
		{
			testCaseName:                      "admin certs",
			input:                             adminCerts,
			expectedCertCount:                 1,
			expectedTrustSettingsPerCertCount: 10,
			expectedError:                     false,
		},
		{
			testCaseName:                      "system certs",
			input:                             systemCerts,
			expectedCertCount:                 7,
			expectedTrustSettingsPerCertCount: 0,
			expectedError:                     false,
		},
		{
			testCaseName:                      "no trust settings",
			input:                             noTrustSettings,
			expectedCertCount:                 1,
			expectedTrustSettingsPerCertCount: 0,
			expectedError:                     false,
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			trustedCerts, err := parseTrustSettingsDump(bytes.NewBuffer(tt.input))
			if tt.expectedError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			require.Equal(t, tt.expectedCertCount, len(trustedCerts))
			for _, cert := range trustedCerts {
				require.Equal(t, tt.expectedTrustSettingsPerCertCount, len(cert.TrustSettings))
			}
		})
	}
}
