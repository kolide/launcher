//go:build windows
// +build windows

package consoleuser

import (
	"testing"
	"time"

	winlsa "github.com/kolide/go-winlsa"
	"github.com/stretchr/testify/require"
)

func Test_shouldIgnoreSession(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName                       string
		sessionData                        *winlsa.LogonSessionData
		usernameInKnownInvalidUsernamesMap bool
		shouldIgnore                       bool
	}{
		{
			testCaseName: "Okta Verify",
			sessionData: &winlsa.LogonSessionData{
				UserName:    "OVService-86103-8588",
				LogonDomain: "SOMEDOMAIN",
			},
			usernameInKnownInvalidUsernamesMap: false,
			shouldIgnore:                       true,
		},
		{
			testCaseName: "Okta Verify but slightly different",
			sessionData: &winlsa.LogonSessionData{
				UserName:    "OVService-38046-0708",
				LogonDomain: "SOMEDOMAIN",
			},
			usernameInKnownInvalidUsernamesMap: false,
			shouldIgnore:                       true,
		},
		{
			testCaseName: "Okta Verify but OVSvc",
			sessionData: &winlsa.LogonSessionData{
				UserName:    "OVSvc-327820240-8998",
				LogonDomain: "SOMEDOMAIN",
			},
			usernameInKnownInvalidUsernamesMap: false,
			shouldIgnore:                       true,
		},
		{
			testCaseName: "Okta Verify but OVSvc, no hyphen",
			sessionData: &winlsa.LogonSessionData{
				UserName:    "OVSvc2137621967-1628",
				LogonDomain: "SOMEDOMAIN",
			},
			usernameInKnownInvalidUsernamesMap: false,
			shouldIgnore:                       true,
		},
		{
			testCaseName: "WsiAccount",
			sessionData: &winlsa.LogonSessionData{
				UserName:    "WsiAccount",
				LogonDomain: "SOMEDOMAIN",
			},
			usernameInKnownInvalidUsernamesMap: false,
			shouldIgnore:                       true,
		},
		{
			testCaseName: "Desktop Windows Manager",
			sessionData: &winlsa.LogonSessionData{
				UserName:    "DWM-3",
				LogonDomain: "Window Manager",
			},
			usernameInKnownInvalidUsernamesMap: false,
			shouldIgnore:                       true,
		},
		{
			testCaseName: "Desktop Windows Manager but slightly different",
			sessionData: &winlsa.LogonSessionData{
				UserName:    "DWM-10",
				LogonDomain: "Window Manager",
			},
			usernameInKnownInvalidUsernamesMap: false,
			shouldIgnore:                       true,
		},
		{
			testCaseName: "defaultuser0",
			sessionData: &winlsa.LogonSessionData{
				UserName:    "defaultuser0",
				LogonDomain: "SOMEDOMAIN",
			},
			usernameInKnownInvalidUsernamesMap: false,
			shouldIgnore:                       true,
		},
		{
			testCaseName: "legitimate user",
			sessionData: &winlsa.LogonSessionData{
				UserName:    "SomeUser",
				LogonDomain: "SOMEDOMAIN",
			},
			usernameInKnownInvalidUsernamesMap: false,
			shouldIgnore:                       false,
		},
		{
			testCaseName: "tracked invalid username",
			sessionData: &winlsa.LogonSessionData{
				UserName:    "SomeUser2",
				LogonDomain: "SOMEDOMAIN",
			},
			usernameInKnownInvalidUsernamesMap: true,
			shouldIgnore:                       true,
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()
			username := usernameFromSessionData(tt.sessionData)
			if tt.usernameInKnownInvalidUsernamesMap {
				knownInvalidUsernamesMapLock.Lock()
				knownInvalidUsernamesMap[username] = struct{}{}
				knownInvalidUsernamesMapLock.Unlock()
			}
			require.Equal(t, tt.shouldIgnore, shouldIgnoreSession(tt.sessionData, username))
		})
	}
}

func Test_updateInvalidUsernameMaps(t *testing.T) {
	t.Parallel()

	invalidTestUsername := "username-Test_updateInvalidUsernameMaps"
	for range maxUsernameLookupFailureCount {
		// Confirm username is not yet in knownInvalidUsernamesMap
		knownInvalidUsernamesMapLock.Lock()
		require.NotContains(t, knownInvalidUsernamesMap, invalidTestUsername)
		knownInvalidUsernamesMapLock.Unlock()

		// Record a lookup failure
		updateInvalidUsernameMaps(invalidTestUsername, invalidUsernameLookupWindowSeconds)
	}

	// Confirm that username is now in knownInvalidUsernamesMap
	knownInvalidUsernamesMapLock.Lock()
	require.Contains(t, knownInvalidUsernamesMap, invalidTestUsername)
	knownInvalidUsernamesMapLock.Unlock()

	// Confirm that username is no longer in potentialInvalidUsernamesMap
	potentialInvalidUsernamesMapLock.Lock()
	require.NotContains(t, potentialInvalidUsernamesMap, invalidTestUsername)
	potentialInvalidUsernamesMapLock.Unlock()
}

func Test_updateInvalidUsernameMaps_RequiresFailuresWithinWindow(t *testing.T) {
	t.Parallel()

	invalidTestUsername := "username-Test_updateInvalidUsernameMaps_RequiresFailuresWithinWindow"
	for range maxUsernameLookupFailureCount * 2 {
		// Confirm username is not yet in knownInvalidUsernamesMap
		knownInvalidUsernamesMapLock.Lock()
		require.NotContains(t, knownInvalidUsernamesMap, invalidTestUsername)
		knownInvalidUsernamesMapLock.Unlock()

		// Record a lookup failure, with window of length 1 second
		updateInvalidUsernameMaps(invalidTestUsername, 1)

		// Now, sleep for the length of the failure window, to ensure that failures don't land in the same window
		time.Sleep(1 * time.Second)
	}

	// Confirm that username is still not in knownInvalidUsernamesMap
	knownInvalidUsernamesMapLock.Lock()
	require.NotContains(t, knownInvalidUsernamesMap, invalidTestUsername)
	knownInvalidUsernamesMapLock.Unlock()

	// Confirm that we do still have some timestamps recorded
	potentialInvalidUsernamesMapLock.Lock()
	require.Contains(t, potentialInvalidUsernamesMap, invalidTestUsername)
	require.Equal(t, maxUsernameLookupFailureCount, len(potentialInvalidUsernamesMap[invalidTestUsername]))
	potentialInvalidUsernamesMapLock.Unlock()
}
