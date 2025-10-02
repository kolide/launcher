package katc

import (
	"testing"

	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func Test_camelToSnake(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName   string
		input          string
		expectedOutput string
	}{
		{
			testCaseName:   "basic camelcase column name",
			input:          "emailAddress",
			expectedOutput: "email_address",
		},
		{
			testCaseName:   "already snake case",
			input:          "email_address",
			expectedOutput: "email_address",
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			outputRows, err := camelToSnake(t.Context(), multislogger.NewNopLogger(), map[string][]byte{
				tt.input: nil,
			})
			require.NoError(t, err)
			require.Contains(t, outputRows, tt.expectedOutput)
		})
	}
}
