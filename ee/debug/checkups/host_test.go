package checkups

import "testing"

func Test_formatUptime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		uptime   uint64
		expected string
	}{
		{name: "1 day", uptime: 86400, expected: "1 day"},
		{name: "just over 1 day", uptime: 86401, expected: "1 day, 1 second"},
		{name: "less than a day", uptime: 82860, expected: "23 hours, 1 minute"},
		{name: "just booted", uptime: 0, expected: "0 seconds"},
		{name: "you should reboot", uptime: 34559999, expected: "399 days, 23 hours, 59 minutes, 59 seconds"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if actual := formatUptime(tt.uptime); actual != tt.expected {
				t.Errorf("formatUptime() = %v, want %v", actual, tt.expected)
			}
		})
	}
}
