package snake

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCamelToSnake(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "already snake case",
			input:    "email_address",
			expected: "email_address",
		},
		{
			name:     "basic camel case",
			input:    "emailAddress",
			expected: "email_address",
		},
		{
			name:     "pascal case",
			input:    "EmailAddress",
			expected: "email_address",
		},
		{
			name:     "single word lowercase",
			input:    "email",
			expected: "email",
		},
		{
			name:     "trailing initialism kept whole",
			input:    "userID",
			expected: "user_id",
		},
		{
			name:     "multi-letter initialism",
			input:    "fetchHTTPResponse",
			expected: "fetch_http_response",
		},
		{
			name:     "leading initialism in pascal case",
			input:    "URLPath",
			expected: "url_path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.expected, CamelToSnake(tt.input))
		})
	}
}

func TestSnakeToCamel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "single word",
			input:    "email",
			expected: "Email",
		},
		{
			name:     "two words",
			input:    "email_address",
			expected: "EmailAddress",
		},
		{
			name:     "service name",
			input:    "launcher_kolide_k2_svc",
			expected: "LauncherKolideK2Svc",
		},
		{
			name:     "watchdog task name",
			input:    "launcher_kolide_k2_watchdog_task",
			expected: "LauncherKolideK2WatchdogTask",
		},
		{
			name:     "initialism segment kept upper",
			input:    "user_id",
			expected: "UserID",
		},
		{
			name:     "oauth exception",
			input:    "use_oauth",
			expected: "UseOAuth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.expected, SnakeToCamel(tt.input))
		})
	}
}

func TestSnakeToCamelLower(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single word stays lower",
			input:    "email",
			expected: "email",
		},
		{
			name:     "subsequent words upper",
			input:    "email_address_book",
			expected: "emailAddressBook",
		},
		{
			name:     "initialism only applied after first segment",
			input:    "id_value",
			expected: "idValue",
		},
		{
			name:     "initialism in later segment",
			input:    "user_id",
			expected: "userID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.expected, SnakeToCamelLower(tt.input))
		})
	}
}

func TestCamelToSnake_RoundTrip(t *testing.T) {
	t.Parallel()

	inputs := []string{
		"launcher_kolide_k2_svc",
		"launcher_kolide_k2_watchdog_task",
		"email_address",
		"user_id",
	}

	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, in, CamelToSnake(SnakeToCamel(in)))
		})
	}
}
