package consoleuser

import (
	"context"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCurrentUids(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
	}{
		{
			name: "happy path",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			uids, err := CurrentUids(context.Background())
			assert.NoError(t, err)

			// in the current CI environment (GitHub Actions) the linux runner
			// does not have a console user, so we expect an empty list
			if os.Getenv("CI") == "true" && runtime.GOOS == "linux" {
				assert.Empty(t, uids)
				return
			}

			assert.GreaterOrEqual(t, len(uids), 1, "should have at least one console user")
		})
	}
}
