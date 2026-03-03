package dataflatten

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProtobuf_BasicMessage(t *testing.T) {
	t.Parallel()

	// Wire format for:
	//   field 1 (varint) = 150
	//   field 2 (string) = "hello"
	//   field 3 (nested message) = { field 1 (varint) = 42 }
	data := []byte{
		0x08, 0x96, 0x01, // field 1, varint 150
		0x12, 0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f, // field 2, "hello"
		0x1a, 0x02, 0x08, 0x2a, // field 3, nested {field 1 = 42}
	}

	rows, err := Protobuf(data)
	require.NoError(t, err)

	expected := []Row{
		{Path: []string{"1"}, Value: "150"},
		{Path: []string{"2"}, Value: "hello"},
		{Path: []string{"3", "1"}, Value: "42"},
	}

	sortRows(rows)
	sortRows(expected)
	require.EqualValues(t, expected, rows)
}

func TestProtobuf_RepeatedField(t *testing.T) {
	t.Parallel()

	// field 2 (varint) repeated: 1, 2, 3
	data := []byte{
		0x10, 0x01, // field 2, varint 1
		0x10, 0x02, // field 2, varint 2
		0x10, 0x03, // field 2, varint 3
	}

	rows, err := Protobuf(data)
	require.NoError(t, err)

	expected := []Row{
		{Path: []string{"2", "0"}, Value: "1"},
		{Path: []string{"2", "1"}, Value: "2"},
		{Path: []string{"2", "2"}, Value: "3"},
	}

	sortRows(rows)
	sortRows(expected)
	require.EqualValues(t, expected, rows)
}

func TestProtobuf_StringField(t *testing.T) {
	t.Parallel()

	// field 1 (string) = "Hello, World!"
	msg := "Hello, World!"
	data := []byte{0x0a, byte(len(msg))}
	data = append(data, []byte(msg)...)

	rows, err := Protobuf(data)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "Hello, World!", rows[0].Value)
	assert.Equal(t, []string{"1"}, rows[0].Path)
}

func TestProtobuf_EmptyInput(t *testing.T) {
	t.Parallel()

	rows, err := Protobuf([]byte{})
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestProtobuf_InvalidData(t *testing.T) {
	t.Parallel()

	// 11 bytes of 0xFF overflows varint decoding
	data := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	_, err := Protobuf(data)
	require.Error(t, err)
}

func TestProtobuf_WithQuery(t *testing.T) {
	t.Parallel()

	data := []byte{
		0x08, 0x96, 0x01, // field 1, varint 150
		0x12, 0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f, // field 2, "hello"
	}

	rows, err := Protobuf(data, WithQuery([]string{"2"}))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "hello", rows[0].Value)
}

func TestProtobuf_NestedQuery(t *testing.T) {
	t.Parallel()

	// Repeated field 3 with nested messages creates an array, so path
	// becomes 3/<index>/1. Use wildcard to match all array elements.
	data := []byte{
		0x1a, 0x02, 0x08, 0x2a, // field 3, nested {field 1 = 42}
		0x1a, 0x02, 0x08, 0x07, // field 3, nested {field 1 = 7}
	}

	rows, err := Protobuf(data, WithQuery([]string{"3", "*", "1"}))
	require.NoError(t, err)
	require.Len(t, rows, 2)

	values := []string{rows[0].Value, rows[1].Value}
	sort.Strings(values)
	assert.Equal(t, []string{"42", "7"}, values)
}

func TestProtobufFile(t *testing.T) {
	t.Parallel()

	testFile := filepath.Join("..", "tables", "protobuf", "test-data", "ws.pb")

	rows, err := ProtobufFile(testFile)
	require.NoError(t, err, "ProtobufFile should decode ws.pb without error")
	assert.NotEmpty(t, rows, "ws.pb should produce at least some flattened rows")

	t.Logf("ProtobufFile produced %d rows from ws.pb", len(rows))

	// Log a sample of the first few rows for visibility
	for i, row := range rows {
		if i >= 20 {
			t.Logf("  ... and %d more rows", len(rows)-20)
			break
		}
		t.Logf("  %s = %s", row.StringPath("/"), row.Value)
	}
}

func TestProtobufFile_NotFound(t *testing.T) {
	t.Parallel()

	_, err := ProtobufFile("/nonexistent/path/to/file.pb")
	require.Error(t, err)
}

func TestDecodeBytesField_Heuristics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    []byte
		expected any
	}{
		{
			name:     "empty bytes",
			input:    []byte{},
			expected: "",
		},
		{
			name:     "printable ascii string",
			input:    []byte("hello world"),
			expected: "hello world",
		},
		{
			name:     "url string",
			input:    []byte("https://example.com/path?q=1"),
			expected: "https://example.com/path?q=1",
		},
		{
			name:     "nested protobuf message",
			input:    []byte{0x08, 0x2a}, // field 1, varint 42
			expected: map[string]any{"1": uint64(42)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := decodeBytesField(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPbBytesLooksLikeString(t *testing.T) {
	t.Parallel()

	assert.True(t, pbBytesLooksLikeString([]byte("hello")))
	assert.True(t, pbBytesLooksLikeString([]byte("line1\nline2")))
	assert.True(t, pbBytesLooksLikeString([]byte("col1\tcol2")))
	assert.False(t, pbBytesLooksLikeString([]byte{0x08, 0x01})) // protobuf tag + varint
	assert.False(t, pbBytesLooksLikeString([]byte{0x00}))        // null byte
	assert.False(t, pbBytesLooksLikeString([]byte{0xFF, 0xFE}))  // invalid UTF-8
}

func sortRows(rows []Row) {
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].StringPath("/") < rows[j].StringPath("/")
	})
}
