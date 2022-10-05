//go:build !linux
// +build !linux

package consoleuser

import (
	"context"
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
			assert.GreaterOrEqual(t, len(uids), 1, "should have at least one console user")
		})
	}
}
