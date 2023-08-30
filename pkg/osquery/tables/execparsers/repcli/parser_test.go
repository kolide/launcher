package repcli

import (
	"bytes"
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"
)

//go:embed test-data/repcli_linux.txt
var repcli_linux_status []byte

//go:embed test-data/repcli_darwin.txt
var repcli_mac_status []byte

func TestParse(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name     string
		input    []byte
		expected map[string]map[string]any
	}{
		{
			name:     "empty input",
			expected: NewStatusTemplate(),
		},
		{
			name:     "malformed input",
			input:    []byte("\n\nGeneral Info:\n\nGarbage: values\nmissing colons\ndouble colon values:: oh my\n\n"),
			expected: NewStatusTemplate(),
		},
		{
			name:  "repcli linux status",
			input: repcli_linux_status,
			expected: map[string]map[string]interface{}{
				"general_info": {
					"devicehash":         "test6b7v9Xo5bX50okW5KABCD+wHxb/YZeSzrZACKo0=",
					"deviceid":           "123453928",
					"quarantine":         "No",
					"sensor_version":     "2.14.0.1234321",
					"sensor_restarts":    "",
					"kernel_type":        "",
					"system_extension":   "",
					"kernel_file_filter": "",
					"background_scan":    "",
					"last_reset":         "",
					"fips_mode":          "",
				},
				"full_disk_access_configurations": {
					"repmgr":           "",
					"system_extension": "",
					"osquery":          "",
					"uninstall_helper": "",
					"uninstall_ui":     "",
				},
				"sensor_status": {
					"status": "Enabled",
					"details": map[string][]string{
						"liveresponse": {
							"NoSession",
							"Enabled",
							"NoKillSwitch",
						},
					},
					"svcstable":                   "",
					"boot_count":                  "",
					"first_boot_after_os_upgrade": "",
					"service_uptime":              "",
					"service_waketime":            "",
				},
				"cloud_status": {
					"registered":         "Yes",
					"server_address":     "https://dev-prod06.example.com",
					"next_check-in":      "",
					"private_logging":    "",
					"next_cloud_upgrade": "",
					"mdm_device_id":      "",
					"platform_type":      "",
					"proxy":              "No",
				},
				"proxy_settings": {
					"proxy_configured": "",
				},
				"enforcement_status": {
					"execution_blocks":     "",
					"network_restrictions": "",
				},
				"rules_status": {
					"policy_name":               "LinuxDefaultPolicy",
					"policy_timestamp":          "02/20/2023",
					"endpoint_standard_product": "",
					"enterprise_edr_product":    "",
					"active_policies":           make(map[string]string, 0),
				},
			},
		},
		{
			name:  "repcli mac status",
			input: repcli_mac_status,
			expected: map[string]map[string]interface{}{
				"general_info": {
					"devicehash":         "",
					"deviceid":           "",
					"quarantine":         "",
					"sensor_version":     "3.7.2.81",
					"sensor_restarts":    "1911",
					"kernel_type":        "System Extension",
					"system_extension":   "Running",
					"kernel_file_filter": "Connected",
					"background_scan":    "Complete",
					"last_reset":         "not set",
					"fips_mode":          "Disabled",
				},
				"full_disk_access_configurations": {
					"repmgr":           "Not Configured",
					"system_extension": "Unknown",
					"osquery":          "Unknown",
					"uninstall_helper": "Unknown",
					"uninstall_ui":     "Unknown",
				},
				"sensor_status": {
					"status": "Enabled",
					"details": map[string][]string{
						"liveresponse": {
							"NoSession",
							"NoKillSwitch",
							"Enabled",
						},
						"fulldiskaccess": {"NotEnabled"},
					},
					"svcstable":                   "Yes",
					"boot_count":                  "103",
					"first_boot_after_os_upgrade": "No",
					"service_uptime":              "155110500 ms",
					"service_waketime":            "37860000 ms",
				},
				"cloud_status": {
					"registered":         "Yes",
					"server_address":     "https://dev-prod05.example.com",
					"next_check-in":      "Now",
					"private_logging":    "Disabled",
					"next_cloud_upgrade": "None",
					"mdm_device_id":      "9FCF04E4-4C8C-45A0-B3EA-053672776382",
					"platform_type":      "CLIENT_ARM64",
					"proxy":              "",
				},
				"proxy_settings": {
					"proxy_configured": "No",
				},
				"enforcement_status": {
					"execution_blocks":     "0",
					"network_restrictions": "0",
				},
				"rules_status": {
					"policy_name":               "Workstations",
					"policy_timestamp":          "08/22/2023 15:19:53",
					"endpoint_standard_product": "Enabled",
					"enterprise_edr_product":    "Enabled",
					"active_policies": map[string]string{
						"sensor_telemetry_reporting_policy_revision[3]": "Enabled(Built-in)",
						"eedr_reporting_revision[18]":                   "Enabled(Manifest)",
						"device_control_reporting_policy_revision[5]":   "Enabled(Manifest)",
						"dc_allow_external_devices_revision[1]":         "Enabled(Manifest)",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := New()
			result, err := p.Parse(bytes.NewReader(tt.input))
			require.NoError(t, err, "unexpected error parsing input")

			require.Equal(t, tt.expected, result)
		})
	}
}
