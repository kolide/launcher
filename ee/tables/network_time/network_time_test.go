//go:build darwin

package network_time

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kolide/launcher/v2/ee/tables/network_time/mocks"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestGenerateNetworkTimeData(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name           string
		execReturnFile string
		want           []map[string]string
	}{
		{
			name:           "network time on",
			execReturnFile: "on.output",
			want: []map[string]string{
				{
					"using_network_time":  "1",
					"network_time_server": "time.apple.com",
				},
			},
		},
		{
			name:           "network time off",
			execReturnFile: "off.output",
			want: []map[string]string{
				{
					"using_network_time":  "0",
					"network_time_server": "time.apple.com",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			execReturn, err := os.ReadFile(filepath.Join("testdata", tt.execReturnFile))
			require.NoError(t, err, "read exec return file")

			executor := mocks.NewExecutor(t)
			executor.On("ExecNetworkTime").Return(execReturn, nil).Once()

			got, err := generateNetworkTimeData(t.Context(), executor, multislogger.NewNopLogger())
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestParseSystemsetupOutput(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name   string
		output string
		want   map[string]string
	}{
		{
			name:   "all settings",
			output: "Network Time: On\nNetwork Time Server: time.apple.com\n",
			want: map[string]string{
				"using_network_time":  "1",
				"network_time_server": "time.apple.com",
			},
		},
		{
			name:   "time server only",
			output: "Network Time Server: time.apple.com\n",
			want:   map[string]string{"network_time_server": "time.apple.com"},
		},
		{
			name:   "no trailing newline",
			output: "Network Time: On",
			want:   map[string]string{"using_network_time": "1"},
		},
		{
			name:   "unknown settings are skipped",
			output: "Wake On Modem: On\n\nNetwork Time: Off\n",
			want:   map[string]string{"using_network_time": "0"},
		},
		{
			name:   "empty output",
			output: "",
			want:   map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, parseSystemsetupOutput([]byte(tt.output)))
		})
	}
}

func TestSanitizeState(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name  string
		state string
		want  string
	}{
		{
			name:  "on",
			state: "On",
			want:  "1",
		},
		{
			name:  "off",
			state: "Off",
			want:  "0",
		},
		{
			name:  "unknown state",
			state: "Maybe",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, sanitizeState(tt.state))
		})
	}
}
