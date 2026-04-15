package secretscan

import (
	"encoding/base64"
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/kolide/launcher/v2/ee/tables/tablehelpers"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func TestGenerate_SkipsKnownFalsePositives(t *testing.T) {
	t.Parallel()

	// This table is missing a _lot_ of the negative tests. This is intentional -- because
	// this is called on every secret, those are covered by the normal tests
	tests := []struct {
		name       string
		secret     string
		expectRows bool
	}{
		{
			name:       "generic api key with randomized b5-like json secret is skipped",
			secret:     randomB5LikeSecret(t, 42, []string{"cty", "data", "enc", "iv", "kid"}),
			expectRows: false,
		},
		{
			name:       "generic api key with missing expected field is not skipped",
			secret:     randomB5LikeSecret(t, 43, []string{"cty", "data", "enc", "iv"}),
			expectRows: true,
		},
		{
			name:       "generic api key with extra field is skipped",
			secret:     randomB5LikeSecret(t, 44, []string{"cty", "data", "enc", "iv", "kid", "notit"}),
			expectRows: true,
		},
		{
			name:       "non matching malformed secret produces no findings",
			secret:     "definitely-not-base64",
			expectRows: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tbl := &Table{
				slogger: multislogger.NewNopLogger(),
			}

			tempDir := t.TempDir()
			targetFile := filepath.Join(tempDir, "target.env")
			fileContent := "KEY=" + tt.secret + "\nKEY_QUOTED=\"" + tt.secret + "\"\n"

			err := os.WriteFile(targetFile, []byte(fileContent), 0600)
			require.NoError(t, err)

			rows, err := tbl.generate(t.Context(), tablehelpers.MockQueryContext(map[string][]string{
				"path": {targetFile},
			}))
			require.NoError(t, err)

			if tt.expectRows {
				require.Equal(t, 2, len(rows), "expected finding to remain after false positive filtering")
				return
			}

			require.Empty(t, rows, "expected finding to be filtered as false positive")
		})
	}
}

func randomB5LikeSecret(t *testing.T, seed int64, keys []string) string {
	t.Helper()

	rng := rand.New(rand.NewSource(seed))

	payload := make(map[string]string, len(keys))
	for _, key := range keys {
		payload[key] = randomAlphaNumeric(rng, 32)
	}

	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err)

	return base64.StdEncoding.EncodeToString(payloadJSON)
}

func randomAlphaNumeric(rng *rand.Rand, n int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	buf := make([]byte, n)
	for i := range n {
		buf[i] = charset[rng.Intn(len(charset))]
	}

	return string(buf)
}
