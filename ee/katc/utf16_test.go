package katc

import (
	"testing"

	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func Test_utf16Decode(t *testing.T) {
	t.Parallel()

	// UTF-16 LE bytes for "1771965616283" (each ASCII char followed by 0x00)
	utf16LETimestamp := []byte{
		0x31, 0x00, 0x37, 0x00, 0x37, 0x00, 0x31, 0x00,
		0x39, 0x00, 0x36, 0x00, 0x35, 0x00, 0x36, 0x00,
		0x31, 0x00, 0x36, 0x00, 0x32, 0x00, 0x38, 0x00,
		0x33, 0x00,
	}

	expectedTimestampDecoded := []byte("1771965616283")

	// UTF-16 LE with BOM for "hello": BOM 0xFF 0xFE + h e l l o
	utf16LEWithBOM := []byte{
		0xFF, 0xFE, 0x68, 0x00, 0x65, 0x00, 0x6C, 0x00, 0x6C, 0x00, 0x6F, 0x00,
	}

	// UTF-16 LE without BOM for "hello"
	utf16LENoBOM := []byte{
		0x68, 0x00, 0x65, 0x00, 0x6C, 0x00, 0x6C, 0x00, 0x6F, 0x00,
	}

	// UTF-16 BE with BOM for "hello": BOM 0xFE 0xFF + h e l l o
	utf16BEWithBOM := []byte{
		0xFE, 0xFF, 0x00, 0x68, 0x00, 0x65, 0x00, 0x6C, 0x00, 0x6C, 0x00, 0x6F,
	}

	// UTF-16 BE without BOM for "hello"
	utf16BENoBOM := []byte{
		0x00, 0x68, 0x00, 0x65, 0x00, 0x6C, 0x00, 0x6C, 0x00, 0x6F,
	}

	// Plain ASCII (return as-is): neither BOM nor null-byte pattern
	plainASCIIHello := []byte("hello")

	tests := []struct {
		name     string
		input    map[string][]byte
		expected map[string][]byte
	}{
		{
			name:     "UTF-16 LE with BOM decodes to string",
			input:    map[string][]byte{"value": utf16LEWithBOM},
			expected: map[string][]byte{"value": plainASCIIHello},
		},
		{
			name:     "UTF-16 BE with BOM decodes to string",
			input:    map[string][]byte{"value": utf16BEWithBOM},
			expected: map[string][]byte{"value": plainASCIIHello},
		},
		{
			name:     "UTF-16 LE timestamp without BOM (2nd byte null heuristic)",
			input:    map[string][]byte{"value": utf16LETimestamp},
			expected: map[string][]byte{"value": expectedTimestampDecoded},
		},
		{
			name:     "UTF-16 LE without BOM (2nd byte null heuristic)",
			input:    map[string][]byte{"value": utf16LENoBOM},
			expected: map[string][]byte{"value": plainASCIIHello},
		},
		{
			name:     "UTF-16 BE without BOM (1st byte null heuristic)",
			input:    map[string][]byte{"value": utf16BENoBOM},
			expected: map[string][]byte{"value": plainASCIIHello},
		},
		{
			name:     "plain ASCII returns as-is",
			input:    map[string][]byte{"value": plainASCIIHello},
			expected: map[string][]byte{"value": plainASCIIHello},
		},
		{
			name:     "empty input returns as-is",
			input:    map[string][]byte{"value": []byte{}},
			expected: map[string][]byte{"value": []byte{}},
		},
		{
			name:     "single byte returns as-is",
			input:    map[string][]byte{"value": []byte{0x41}},
			expected: map[string][]byte{"value": []byte{0x41}},
		},
		{
			name:     "multiple keys with mixed encoding",
			input:    map[string][]byte{"key": []byte{0x31, 0x00}, "value": utf16LETimestamp},
			expected: map[string][]byte{"key": []byte("1"), "value": []byte("1771965616283")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			results, err := utf16Decode(t.Context(), multislogger.NewNopLogger(), tt.input)
			require.NoError(t, err)
			require.Len(t, results, 1)
			for k, want := range tt.expected {
				require.Contains(t, results[0], k)
				require.Equal(t, want, results[0][k])
			}
		})
	}
}
