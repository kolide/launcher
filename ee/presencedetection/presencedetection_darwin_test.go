package presencedetection

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testPresenceEnvVar = "LAUNCHER_TEST_PRESENCE"

// Since there is no way to test user presence in a CI / automated fashion,
// these test are expected to be run manually via cmd line when needed.

// To test this run
//
// LAUNCHER_TEST_PRESENCE=true go test ./ee/presencedetection/ -run Test_detectSuccess
//
// then successfully auth with the pop up
func Test_detectSuccess(t *testing.T) {
	t.Parallel()

	if os.Getenv(testPresenceEnvVar) == "" {
		t.Skip("Skipping Test_detectSuccess")
	}

	success, err := Detect("IS TRYING TO TEST SUCCESS, PLEASE AUTHENTICATE", DetectionTimeout)
	require.NoError(t, err, "should not get an error on successful detect")
	assert.True(t, success, "should be successful")
}

// To test this run
//
// LAUNCHER_TEST_PRESENCE=true go test ./ee/presencedetection/ -run Test_detectCancel
//
// then cancel the biometric auth that pops up
func Test_detectCancel(t *testing.T) {
	t.Parallel()

	if os.Getenv(testPresenceEnvVar) == "" {
		t.Skip("Skipping test_biometricDetectCancel")
	}

	success, err := Detect("IS TRYING TO TEST CANCEL, PLEASE PRESS CANCEL", DetectionTimeout)
	require.Error(t, err, "should get an error on failed detect")
	assert.False(t, success, "should not be successful")
}

// To test this run
//
// LAUNCHER_TEST_PRESENCE=true go test ./ee/presencedetection/ -run Test_timeout
//
// then do not press anything on the biometric auth that pops up
func Test_timeout(t *testing.T) {
	t.Parallel()

	if os.Getenv(testPresenceEnvVar) == "" {
		t.Skip("Skipping test_biometricDetectCancel")
	}

	success, err := Detect("IS TRYING TO TEST TIMEOUT, PLEASE DO NOTHING", 3*time.Second)
	require.Error(t, err, "should get an error on failed detect")
	assert.False(t, success, "should not be successful")
}
