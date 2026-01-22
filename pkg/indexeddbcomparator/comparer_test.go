package indexeddbcomparator

import (
	"log/slog"
	"testing"

	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
)

func TestIdbCmp1Comparer_Compare(t *testing.T) {
	t.Parallel()
	var tests = []struct {
		name             string
		keyA             []byte
		keyB             []byte
		expectedResult   int
		expectErrMatches string
	}{
		{
			name:             "empty key",
			keyA:             []byte{},
			keyB:             []byte{0x00, 0x00, 0x00, 0x00, 0x00},
			expectedResult:   0,
			expectErrMatches: "invalid empty key",
		},
		{
			name:             "insufficient length key",
			keyA:             []byte{0x00, 0x00},
			keyB:             []byte{0x00, 0x00, 0x00, 0x00, 0x00},
			expectedResult:   0,
			expectErrMatches: "insufficient length",
		},
		{
			name:             "invalid prefix type byte",
			keyA:             []byte{0x02, 0x00, 0x00, 0x00, 0x02, 0x01, 0xFF},
			keyB:             []byte{0x02, 0x00, 0x00, 0x00, 0x02, 0x01, 0xFF},
			expectedResult:   0,
			expectErrMatches: "invalid key prefix type byte",
		},
		{
			name:             "valid data store prefix type - a should be less",
			keyA:             []byte{0, 2, 1, 1, 3, 0, 0, 0, 0, 208, 232, 0, 65},
			keyB:             []byte{0, 2, 1, 1, 3, 0, 0, 0, 0, 216, 232, 0, 65},
			expectedResult:   -1,
			expectErrMatches: "",
		},
		{
			name:             "valid data store prefix type - b should be less",
			keyA:             []byte{0, 2, 1, 1, 3, 0, 0, 0, 0, 216, 232, 0, 65},
			keyB:             []byte{0, 2, 1, 1, 3, 0, 0, 0, 0, 208, 232, 0, 65},
			expectedResult:   1,
			expectErrMatches: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var logBytes threadsafebuffer.ThreadSafeBuffer

			slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
				AddSource: false,
				Level:     slog.LevelDebug,
			}))

			comparer := &idbCmp1Comparer{
				slogger: slogger,
			}

			result := comparer.Compare(tt.keyA, tt.keyB)
			require.Equal(t, tt.expectedResult, result)
			if tt.expectErrMatches != "" {
				require.Contains(t, logBytes.String(), tt.expectErrMatches)
			}
		})
	}
}

func TestIdbCmp1Comparer_compareDouble(t *testing.T) {
	t.Parallel()
	var tests = []struct {
		name             string
		keyA             []byte
		keyB             []byte
		expectedResult   int
		expectErrMatches string
	}{
		{
			name:             "insufficient length key",
			keyA:             []byte{0x00, 0x00},
			keyB:             []byte{0x00, 0x00, 0x00, 0x00, 0x00},
			expectedResult:   0,
			expectErrMatches: "invalid keys provided for compareDouble",
		},
		{
			name:             "valid float bits - a should be less",
			keyA:             []byte{64, 94, 220, 204, 204, 204, 204, 205}, // 123.45
			keyB:             []byte{64, 95, 35, 215, 10, 61, 112, 164},    // 124.56
			expectedResult:   -1,
			expectErrMatches: "",
		},
		{
			name:             "valid float bits - b should be less",
			keyA:             []byte{64, 95, 35, 215, 10, 61, 112, 164},    // 124.56
			keyB:             []byte{64, 94, 220, 204, 204, 204, 204, 205}, // 123.45
			expectedResult:   1,
			expectErrMatches: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var logBytes threadsafebuffer.ThreadSafeBuffer

			slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
				AddSource: false,
				Level:     slog.LevelDebug,
			}))

			comparer := &idbCmp1Comparer{
				slogger: slogger,
			}

			_, _, result, err := comparer.compareDouble(tt.keyA, tt.keyB)
			require.Equal(t, tt.expectedResult, result)
			if tt.expectErrMatches != "" {
				require.Error(t, err)
				require.Contains(t, logBytes.String(), tt.expectErrMatches)
			}
		})
	}
}

func TestIdbCmp1Comparer_compareStringWithLength(t *testing.T) {
	t.Parallel()
	// generated these test cases by taking a live indexeddb (previously showing iteration issues), and making a sample test over in katc table_test.go which would iterate all keys.
	// Then sticking a debugger into the comparer logic and stepping through until we found two stringWithLength values that should not sort equally-
	// verified the output was correct and then turned those into byte arrays and swapped the order across tests to show consistent/correct comparisons
	var tests = []struct {
		name             string
		keyA             []byte
		keyB             []byte
		expectedResult   int
		expectErrMatches string
	}{
		{
			name:             "valid string with length - a should be less",
			keyA:             []byte{24, 0, 54, 0, 56, 0, 99, 0, 99, 0, 56, 0, 50, 0, 52, 0, 98, 0, 100, 0, 102, 0, 55, 0, 99, 0, 52, 0, 57, 0, 97, 0, 49, 0, 102, 0, 54, 0, 102, 0, 51, 0, 97, 0, 51, 0, 102, 0, 97},
			keyB:             []byte{24, 0, 54, 0, 56, 0, 100, 0, 49, 0, 97, 0, 49, 0, 98, 0, 55, 0, 99, 0, 99, 0, 49, 0, 99, 0, 100, 0, 53, 0, 97, 0, 97, 0, 51, 0, 54, 0, 56, 0, 97, 0, 50, 0, 102, 0, 99, 0, 55},
			expectedResult:   -1,
			expectErrMatches: "",
		},
		{
			name:             "valid string with length - b should be less",
			keyA:             []byte{24, 0, 54, 0, 56, 0, 100, 0, 49, 0, 97, 0, 49, 0, 98, 0, 55, 0, 99, 0, 99, 0, 49, 0, 99, 0, 100, 0, 53, 0, 97, 0, 97, 0, 51, 0, 54, 0, 56, 0, 97, 0, 50, 0, 102, 0, 99, 0, 55},
			keyB:             []byte{24, 0, 54, 0, 56, 0, 99, 0, 99, 0, 56, 0, 50, 0, 52, 0, 98, 0, 100, 0, 102, 0, 55, 0, 99, 0, 52, 0, 57, 0, 97, 0, 49, 0, 102, 0, 54, 0, 102, 0, 51, 0, 97, 0, 51, 0, 102, 0, 97},
			expectedResult:   1,
			expectErrMatches: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var logBytes threadsafebuffer.ThreadSafeBuffer

			slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
				AddSource: false,
				Level:     slog.LevelDebug,
			}))

			comparer := &idbCmp1Comparer{
				slogger: slogger,
			}

			_, _, result, err := comparer.compareStringWithLength(tt.keyA, tt.keyB)
			require.Equal(t, tt.expectedResult, result)
			if tt.expectErrMatches != "" {
				require.Error(t, err)
				require.Contains(t, logBytes.String(), tt.expectErrMatches)
			}
		})
	}
}
