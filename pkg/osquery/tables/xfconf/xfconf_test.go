//go:build !windows
// +build !windows

package xfconf

import (
	"os/user"
)

type parseTestCase struct {
	channelName    string
	user           *user.User
	input          string
	expectedOutput []map[string]string
}

func getParseTestCases() []parseTestCase {
	testUsername := "headless"
	testUser := user.User{
		Name: testUsername,
	}

	return []parseTestCase{
		{
			channelName: "xfce4-power-manager",
			user:        &testUser,
			input: `/xfce4-power-manager/lock-screen-suspend-hibernate  true
/xfce4-power-manager/power-button-action            3`,
			expectedOutput: []map[string]string{
				{
					"channel":  "xfce4-power-manager",
					"key":      "/xfce4-power-manager/lock-screen-suspend-hibernate",
					"value":    "true",
					"username": testUsername,
				},
				{
					"channel":  "xfce4-power-manager",
					"key":      "/xfce4-power-manager/power-button-action",
					"value":    "3",
					"username": testUsername,
				},
			},
		},
		{
			channelName: "thunar-volman",
			user:        &testUser,
			input: `/autobrowse/enabled        false
/automount-drives/enabled  false
/automount-media/enabled   false
/autoopen/enabled          false
/autorun/enabled           true`,
			expectedOutput: []map[string]string{
				{
					"channel":  "thunar-volman",
					"key":      "/autobrowse/enabled",
					"value":    "false",
					"username": testUsername,
				},
				{
					"channel":  "thunar-volman",
					"key":      "/automount-drives/enabled",
					"value":    "false",
					"username": testUsername,
				},
				{
					"channel":  "thunar-volman",
					"key":      "/automount-media/enabled",
					"value":    "false",
					"username": testUsername,
				},
				{
					"channel":  "thunar-volman",
					"key":      "/autoopen/enabled",
					"value":    "false",
					"username": testUsername,
				},
				{
					"channel":  "thunar-volman",
					"key":      "/autorun/enabled",
					"value":    "true",
					"username": testUsername,
				},
			},
		},
		{
			channelName: "xfce4-session",
			user:        &testUser,
			input: `/general/FailsafeSessionName          Failsafe
/general/LockCommand                  
/sessions/Failsafe/Client0_Command    <<UNSUPPORTED>>
/sessions/Failsafe/Client0_PerScreen  false
/sessions/Failsafe/Client0_Priority   15
/sessions/Failsafe/Count              1
/sessions/Failsafe/IsFailsafe         true`,
			expectedOutput: []map[string]string{
				{
					"channel":  "xfce4-session",
					"key":      "/general/FailsafeSessionName",
					"value":    "Failsafe",
					"username": testUsername,
				},
				{
					"channel":  "xfce4-session",
					"key":      "/general/LockCommand",
					"value":    "",
					"username": testUsername,
				},
				{
					"channel":  "xfce4-session",
					"key":      "/sessions/Failsafe/Client0_Command",
					"value":    "<<UNSUPPORTED>>",
					"username": testUsername,
				},
				{
					"channel":  "xfce4-session",
					"key":      "/sessions/Failsafe/Client0_PerScreen",
					"value":    "false",
					"username": testUsername,
				},
				{
					"channel":  "xfce4-session",
					"key":      "/sessions/Failsafe/Client0_Priority",
					"value":    "15",
					"username": testUsername,
				},
				{
					"channel":  "xfce4-session",
					"key":      "/sessions/Failsafe/Count",
					"value":    "1",
					"username": testUsername,
				},
				{
					"channel":  "xfce4-session",
					"key":      "/sessions/Failsafe/IsFailsafe",
					"value":    "true",
					"username": testUsername,
				},
			},
		},
	}
}
