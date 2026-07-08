package dtainfoconsumer

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jwt "github.com/golang-jwt/jwt/v5"
	typesmocks "github.com/kolide/launcher/v2/ee/agent/types/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestUpdate(t *testing.T) {
	t.Parallel()

	const testMunemo = "test"
	happyBlob := signTestJWT(t, jwt.MapClaims{"munemo": testMunemo})

	tests := []struct {
		name    string
		payload string
		munemo  string // when set, expect this claim
	}{
		{
			name:    "empty payload",
			payload: `{}`,
		},
		{
			name:    "null dta blob",
			payload: `{"dta_blob": null}`,
		},
		{
			name:    "invalid json",
			payload: `not json`,
		},
		{
			name:    "invalid jwt",
			payload: `{"dta_blob": "not-a-jwt"}`,
		},
		{
			name:    "jwt missing munemo claim",
			payload: dtaPayloadFor(signTestJWT(t, jwt.MapClaims{"other": "x"})),
		},
		{
			name:    "munemo claim not a string",
			payload: dtaPayloadFor(signTestJWT(t, jwt.MapClaims{"munemo": 42})),
		},
		{
			name:    "happy path",
			payload: dtaPayloadFor(happyBlob),
			munemo:  testMunemo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rootDir := t.TempDir()
			consumer := newTestConsumer(t, rootDir)

			err := consumer.Update(strings.NewReader(tt.payload))
			require.NoError(t, err) // even failures are discarded to prevent retries of bad data

			if tt.munemo == "" {
				entries, err := os.ReadDir(rootDir)
				require.NoError(t, err)
				require.Empty(t, entries)
				return
			}

			contents, err := os.ReadFile(filepath.Join(rootDir, dtaFilePrefix+tt.munemo+dtaFileSuffix))
			require.NoError(t, err)
			require.Equal(t, happyBlob, string(contents))
		})
	}
}

func TestUpdateReplacesExistingFile(t *testing.T) {
	t.Parallel()

	const testMunemo = "test-munemo"
	rootDir := t.TempDir()
	consumer := newTestConsumer(t, rootDir)

	firstBlob := signTestJWT(t, jwt.MapClaims{"munemo": testMunemo, "v": 1})
	secondBlob := signTestJWT(t, jwt.MapClaims{"munemo": testMunemo, "v": 2})
	require.NotEqual(t, firstBlob, secondBlob)

	for _, blob := range []string{firstBlob, secondBlob} {
		require.NoError(t, consumer.Update(strings.NewReader(dtaPayloadFor(blob))))
	}

	contents, err := os.ReadFile(consumer.dtaFilePath(testMunemo))
	require.NoError(t, err)
	require.Equal(t, secondBlob, string(contents))
}

// A fatal write error is returned from update, with the goal of allowing retries
// for transient issues.
func TestUpdateFailsOnBadFile(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	consumer := newTestConsumer(t, rootDir)
	path := consumer.dtaFilePath("test")
	require.NoError(t, os.Mkdir(path, 0644))

	blob := signTestJWT(t, jwt.MapClaims{"munemo": "test", "v": 1})
	require.Error(t, consumer.Update(strings.NewReader(dtaPayloadFor(blob))))
}

func newTestConsumer(t *testing.T, rootDir string) *DTAInfoConsumer {
	mockSack := typesmocks.NewKnapsack(t)
	mockSack.On("Slogger").Return(slog.New(slog.DiscardHandler))
	mockSack.On("RootDirectory").Return(rootDir).Maybe()
	return New(mockSack)
}

func dtaPayloadFor(blob string) string {
	return fmt.Sprintf(`{"dta_blob": %q}`, blob)
}

// Just signs with a throwaway key; we aren't sending keys down to verify yet
// outside tests anyway.
func signTestJWT(t *testing.T, claims jwt.MapClaims) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte("test-key"))
	require.NoError(t, err)
	return signed
}
