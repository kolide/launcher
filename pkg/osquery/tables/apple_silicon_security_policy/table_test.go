package apple_silicon_security_policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ParseStatusErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		status         string
		expectedResult map[string]string
	}{
		{
			name: "no data",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := parseStatus([]byte(tt.status))
			require.Error(t, err, "parseStatus")
		})
	}
}

func Test_ParseStatusUnexpected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		status         string
		expectedResult map[string]string
	}{
		{
			name:   "bad output",
			status: "\n\n\n\n",
		},
		{
			name:   "no volume group",
			status: "This utility is not meant for normal users or even sysadmins.",
		},
		{
			name: "only volume group",
			status: `Use at your own risk!
			Local policy for volume group 5D0D176D-E8CC-**REDACTED**:`,
		},
		{
			name: "no column data",
			status: `Use at your own risk!
			Local policy for volume group 5D0D176D-E8CC-**REDACTED**:
			OS Policy:
			Environment:`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := parseStatus([]byte(tt.status))
			require.NoError(t, err, "parseStatus")
			require.Empty(t, data)
		})
	}
}

func Test_ParseStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		status         string
		expectedResult []map[string]string
	}{
		{
			name: "one boot volume",
			status: `This utility is not meant for normal users or even sysadmins.
			It provides unabstracted access to capabilities which are normally handled for the user automatically when changing the security policy through GUIs such as the Startup Security Utility in macOS Recovery ("recoveryOS").
			It is possible to make your system security much weaker and therefore easier to compromise using this tool.
			This tool is not to be used in production environments.
			It is possible to render your system unbootable with this tool.
			It should only be used to understand how the security of Apple Silicon Macs works.
			Use at your own risk!
			
			
			Local policy for volume group 1234567890:
			OS environment:
			OS Type                                       : macOS
			Local Policy Nonce Hash                 (lpnh): 0987654321
			User-allowed MDM Control:    Disabled   (smb3): absent`,
			expectedResult: []map[string]string{
				{"volume_group": "1234567890", "property": "OS_Type", "mode": "", "code": "", "value": "macOS"},
				{"volume_group": "1234567890", "property": "Local_Policy_Nonce_Hash", "mode": "", "code": "lpnh", "value": "0987654321"},
				{"volume_group": "1234567890", "property": "User-allowed_MDM_Control", "mode": "Disabled", "code": "smb3", "value": "absent"},
			},
		},
		{
			name: "multiple boot volumes",
			status: `This utility is not meant for normal users or even sysadmins.
			It provides unabstracted access to capabilities which are normally handled for the user automatically when changing the security policy through GUIs such as the Startup Security Utility in macOS Recovery ("recoveryOS").
			It is possible to make your system security much weaker and therefore easier to compromise using this tool.
			This tool is not to be used in production environments.
			It is possible to render your system unbootable with this tool.
			It should only be used to understand how the security of Apple Silicon Macs works.
			Use at your own risk!
			
			
			Local policy for volume group 123:
			OS environment:
			Boot Args Filtering Status:  Enabled    (sip3): absent
			
			Local policy for volume group 456:
			OS environment:
			Boot Args Filtering Status:  Enabled    (sip3): absent`,
			expectedResult: []map[string]string{
				{"volume_group": "123", "property": "Boot_Args_Filtering_Status", "mode": "Enabled", "code": "sip3", "value": "absent"},
				{"volume_group": "456", "property": "Boot_Args_Filtering_Status", "mode": "Enabled", "code": "sip3", "value": "absent"},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := parseStatus([]byte(tt.status))
			require.NoError(t, err, "parseStatus")

			assert.Equal(t, tt.expectedResult, data)
		})
	}
}
