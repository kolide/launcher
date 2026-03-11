package katc

import (
	"testing"

	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	// We have this exception because goleveldb, on startup, calls mpoolDrain in its own goroutine;
	// on shutdown, mpoolDrain takes up to a second to terminate. It will terminate, so this seems like
	// an okay exception to add.
	// If we wanted to address this directly, we could update goleveldb to add mpoolDrain to the
	// db.closeW waitgroup.
	goleak.VerifyTestMain(m, goleak.IgnoreTopFunction("github.com/kolide/goleveldb/leveldb.(*DB).mpoolDrain"))
}

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
