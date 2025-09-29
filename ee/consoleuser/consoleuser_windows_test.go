//go:build windows
// +build windows

package consoleuser

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func BenchmarkCurrentUids(b *testing.B) {
	// Report memory allocations
	b.ReportAllocs()

	for range b.N {
		_, err := CurrentUids(context.Background())
		assert.NoError(b, err)
	}
}
