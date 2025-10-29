//go:build windows
// +build windows

package consoleuser

import (
	"testing"

	winlsa "github.com/kolide/go-winlsa"
	"github.com/stretchr/testify/require"
)

func Test_shouldIgnoreSession(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName string
		sessionData  *winlsa.LogonSessionData
		shouldIgnore bool
	}{
		{
			testCaseName: "Okta Verify",
			sessionData: &winlsa.LogonSessionData{
				UserName:    "OVService-86103-8588",
				LogonDomain: "SOMEDOMAIN",
			},
			shouldIgnore: true,
		},
		{
			testCaseName: "Okta Verify but slightly different",
			sessionData: &winlsa.LogonSessionData{
				UserName:    "OVService-38046-0708",
				LogonDomain: "SOMEDOMAIN",
			},
			shouldIgnore: true,
		},
		{
			testCaseName: "WsiAccount",
			sessionData: &winlsa.LogonSessionData{
				UserName:    "WsiAccount",
				LogonDomain: "SOMEDOMAIN",
			},
			shouldIgnore: true,
		},
		{
			testCaseName: "Desktop Windows Manager",
			sessionData: &winlsa.LogonSessionData{
				UserName:    "DWM-3",
				LogonDomain: "Window Manager",
			},
			shouldIgnore: true,
		},
		{
			testCaseName: "Desktop Windows Manager but slightly different",
			sessionData: &winlsa.LogonSessionData{
				UserName:    "DWM-10",
				LogonDomain: "Window Manager",
			},
			shouldIgnore: true,
		},
		{
			testCaseName: "legitimate user",
			sessionData: &winlsa.LogonSessionData{
				UserName:    "SomeUser",
				LogonDomain: "SOMEDOMAIN",
			},
			shouldIgnore: false,
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.shouldIgnore, shouldIgnoreSession(tt.sessionData))
		})
	}
}
