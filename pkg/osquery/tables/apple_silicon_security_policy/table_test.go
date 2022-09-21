package apple_silicon_security_policy

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ParseBootPoliciesOutputUnexpected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		status         string
		expectedResult map[string]string
		queryClause    []string
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

			o := parseBootPoliciesOutput(bytes.NewReader([]byte(tt.status)))
			require.Empty(t, o)
		})
	}
}

const oneBootVol = `This utility is not meant for normal users or even sysadmins.
It provides unabstracted access to capabilities which are normally handled for the user automatically when changing the security policy through GUIs such as the Startup Security Utility in macOS Recovery ("recoveryOS").
It is possible to make your system security much weaker and therefore easier to compromise using this tool.
This tool is not to be used in production environments.
It is possible to render your system unbootable with this tool.
It should only be used to understand how the security of Apple Silicon Macs works.
Use at your own risk!


Local policy for volume group 1234567890:
OS environment:
OS Type                                       : macOS`

const multipleBootVol = `This utility is not meant for normal users or even sysadmins.
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
Boot Args Filtering Status:  Enabled    (sip3): absent`

const failedSecondVol = `This utility is not meant for normal users or even sysadmins.
It provides unabstracted access to capabilities which are normally handled for the user automatically when changing the security policy through GUIs such as the Startup Security Utility in macOS Recovery ("recoveryOS").
It is possible to make your system security much weaker and therefore easier to compromise using this tool.
This tool is not to be used in production environments.
It is possible to render your system unbootable with this tool.
It should only be used to understand how the security of Apple Silicon Macs works.
Use at your own risk!


Local policy for volume group 5D0D176D-E8CC-4E8B-815C-444A679390BD:
OS environment:
OS Type                                       : macOS

Local policy:
Security Domain                         (SDOM): 0x1
Production Status                       (CPRO): 1
Can't get local policy for Volume Group UUID 5EADF322-3C85-4DF8-8516-0B556700CD5E
Failed to display local policy for volume group 5EADF322-3C85-4DF8-8516-0B556700CD5E`

func Test_ParsePoliciesTable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		status         string
		expectedResult map[string]interface{}
		queryClause    []string
	}{
		{
			name:           "one boot volume",
			status:         oneBootVol,
			expectedResult: map[string]interface{}{"1234567890": []map[string]interface{}{{"OS_Type": map[string]interface{}{"code": "", "mode": "", "value": "macOS"}}}},
		},
		{
			name:           "multiple boot volumes",
			status:         multipleBootVol,
			expectedResult: map[string]interface{}{"123": []map[string]interface{}{{"Boot_Args_Filtering_Status": map[string]interface{}{"code": "sip3", "mode": "Enabled", "value": "absent"}}}, "456": []map[string]interface{}{{"Boot_Args_Filtering_Status": map[string]interface{}{"code": "sip3", "mode": "Enabled", "value": "absent"}}}},
		},
		{
			name:           "failed second volumes",
			status:         failedSecondVol,
			expectedResult: map[string]interface{}{"5D0D176D-E8CC-4E8B-815C-444A679390BD": []map[string]interface{}{{"OS_Type": map[string]interface{}{"code": "", "mode": "", "value": "macOS"}}, {"Security_Domain": map[string]interface{}{"code": "SDOM", "mode": "", "value": "0x1"}}, {"Production_Status": map[string]interface{}{"code": "CPRO", "mode": "", "value": "1"}}}},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := parseBootPoliciesOutput(bytes.NewReader([]byte(tt.status)))
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func Test_ParsePolicyRow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		line           string
		expectedResult map[string]interface{}
	}{
		{
			name: "empty line",
		},
		{
			name: "no columns",
			line: "data with no colon separators",
		},
		{
			name: "only one column",
			line: "Policy:",
		},
		{
			name:           "two columns",
			line:           "Signature Type                                : BAA",
			expectedResult: map[string]interface{}{"Signature_Type": map[string]interface{}{"value": "BAA", "mode": "", "code": ""}},
		},
		{
			name:           "two columns with code",
			line:           "Local Policy Nonce Hash                 (lpnh): A8D3EC575A03E7F58**REDACTED**",
			expectedResult: map[string]interface{}{"Local_Policy_Nonce_Hash": map[string]interface{}{"value": "A8D3EC575A03E7F58**REDACTED**", "mode": "", "code": "lpnh"}},
		},
		{
			name:           "two columns with code missing last value",
			line:           "Auxiliary Kernel Cache Image4 Hash      (auxi): ",
			expectedResult: map[string]interface{}{"Auxiliary_Kernel_Cache_Image4_Hash": map[string]interface{}{"value": "", "mode": "", "code": "auxi"}},
		},
		{
			name:           "three columns",
			line:           "3rd Party Kexts Status:      Disabled   (smb2): absent",
			expectedResult: map[string]interface{}{"3rd_Party_Kexts_Status": map[string]interface{}{"value": "absent", "mode": "Disabled", "code": "smb2"}},
		},
		{
			name:           "three columns missing last value",
			line:           "Security Mode:               Full       (smb0): ",
			expectedResult: map[string]interface{}{"Security_Mode": map[string]interface{}{"value": "", "mode": "Full", "code": "smb0"}},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := parsePolicyRow(tt.line)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}
