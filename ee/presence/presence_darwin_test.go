package presence

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testPresenceEnvVar = "launcher_test_presence"

// Since there is no way to test user presence in a CI / automated fashion,
// these test are expected to be run manually via cmd line when needed.

// To test this run
//
// launcher_test_presence=true go test ./ee/presence/ -run Test_biometricDetectSuccess
//
// then successfully auth with the pop up
func Test_biometricDetectSuccess(t *testing.T) {
	t.Parallel()

	if os.Getenv(testPresenceEnvVar) == "" {
		t.Skip("Skipping test_biometricDetectSuccess")
	}

	success, err := detect("IS TRYING TO TEST SUCCESS, PLEASE AUTHENTICATE")
	require.NoError(t, err, "should not get an error on successful detect")
	assert.True(t, success, "should be successful")
}

// To test this run
//
// launcher_test_presence=true go test ./ee/presence/ -run Test_biometricDetectCancel
//
// then cancel the biometric auth that pops up
func Test_biometricDetectCancel(t *testing.T) {
	t.Parallel()

	if os.Getenv(testPresenceEnvVar) == "" {
		t.Skip("Skipping test_biometricDetectCancel")
	}

	success, err := detect("IS TRYING TO TEST CANCEL, PLEASE PRESS CANCEL")
	require.Error(t, err, "should get an error on failed detect")
	assert.False(t, success, "should not be successful")
}
