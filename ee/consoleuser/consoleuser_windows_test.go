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

func BenchmarkCurrentUidsViaQuser(b *testing.B) {
	// Report memory allocations
	b.ReportAllocs()

	for range b.N {
		_, err := CurrentUidsViaQuser(context.Background())
		assert.NoError(b, err)
	}
}

func Benchmark_usernameToSIDMap(b *testing.B) {
	// Report memory allocations
	b.ReportAllocs()

	for range b.N {
		_, err := usernameToSIDMap(context.Background())
		assert.NoError(b, err)
	}
}
