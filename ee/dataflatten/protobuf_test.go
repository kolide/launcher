package dataflatten

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// simplePBData is the wire format for a Simple message:
//
//	{name: "test", id: 42, email: "test@example.com"}
var simplePBData = []byte{
	0x0a, 0x04, 't', 'e', 's', 't', // field 1 string "test"
	0x10, 0x2a, // field 2 varint 42
	0x1a, 0x10, 't', 'e', 's', 't', '@', 'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm', // field 3 string "test@example.com"
}

const simpleProtoSchema = `syntax = "proto3";
message Simple {
  string name = 1;
  int32 id = 2;
  string email = 3;
}

message Address {
  string street = 1;
  string city = 2;
  string state = 3;
}

message Person {
  string name = 1;
  int32 id = 2;
  Address address = 3;
  repeated string tags = 4;
}
`

// personPBData is the wire format for a Person message:
//
//	{name: "Alice", id: 1, address: {street: "123 Main St", city: "Springfield", state: "IL"}, tags: ["admin", "user"]}
var personPBData = func() []byte {
	// Inner Address: {street: "123 Main St", city: "Springfield", state: "IL"}
	address := []byte{
		0x0a, 0x0b, '1', '2', '3', ' ', 'M', 'a', 'i', 'n', ' ', 'S', 't', // field 1 string "123 Main St"
		0x12, 0x0b, 'S', 'p', 'r', 'i', 'n', 'g', 'f', 'i', 'e', 'l', 'd', // field 2 string "Springfield"
		0x1a, 0x02, 'I', 'L', // field 3 string "IL"
	}
	// Outer Person
	data := []byte{
		0x0a, 0x05, 'A', 'l', 'i', 'c', 'e', // field 1 string "Alice"
		0x10, 0x01, // field 2 varint 1
		0x1a, byte(len(address)), // field 3 bytes (nested Address)
	}
	data = append(data, address...)
	data = append(data,
		0x22, 0x05, 'a', 'd', 'm', 'i', 'n', // field 4 string "admin"
		0x22, 0x04, 'u', 's', 'e', 'r', // field 4 string "user"
	)
	return data
}()

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

// TestProtobuf_Base64 verifies that base64-encoded protobuf data is
// transparently decoded by the Protobuf bytes function.
func TestProtobuf_Base64(t *testing.T) {
	t.Parallel()

	encoded := []byte(base64.StdEncoding.EncodeToString(simplePBData))

	rows, err := Protobuf(encoded)
	require.NoError(t, err)
	require.Len(t, rows, 3)

	rowMap := make(map[string]string)
	for _, r := range rows {
		rowMap[r.StringPath("/")] = r.Value
	}
	assert.Equal(t, "test", rowMap["1"])
	assert.Equal(t, "42", rowMap["2"])
	assert.Equal(t, "test@example.com", rowMap["3"])
}

// TestProtobuf_SchemalessVsSchemaAware compares output of the same protobuf
// data decoded with and without a .proto schema.
func TestProtobuf_SchemalessVsSchemaAware(t *testing.T) {
	t.Parallel()

	// Schema-less: field numbers as keys
	schemalessRows, err := Protobuf(simplePBData)
	require.NoError(t, err)
	require.Len(t, schemalessRows, 3)

	schemalessMap := make(map[string]string)
	for _, r := range schemalessRows {
		schemalessMap[r.StringPath("/")] = r.Value
	}
	assert.Equal(t, "test", schemalessMap["1"])
	assert.Equal(t, "42", schemalessMap["2"])
	assert.Equal(t, "test@example.com", schemalessMap["3"])

	// Schema-aware: field names as keys
	schemaFn := ProtobufWithSchema([]byte(simpleProtoSchema), "Simple")
	schemaRows, err := schemaFn(simplePBData)
	require.NoError(t, err)
	require.Len(t, schemaRows, 3)

	schemaMap := make(map[string]string)
	for _, r := range schemaRows {
		schemaMap[r.StringPath("/")] = r.Value
	}
	assert.Equal(t, "test", schemaMap["name"])
	assert.Equal(t, "42", schemaMap["id"])
	assert.Equal(t, "test@example.com", schemaMap["email"])
}

// TestProtobuf_SchemaAwareBase64 verifies schema-aware decoding with
// base64-encoded input.
func TestProtobuf_SchemaAwareBase64(t *testing.T) {
	t.Parallel()

	encoded := base64.StdEncoding.EncodeToString(simplePBData)
	schemaFn := ProtobufWithSchema([]byte(simpleProtoSchema), "Simple")
	rows, err := schemaFn([]byte(encoded))
	require.NoError(t, err)
	require.Len(t, rows, 3)

	rowMap := make(map[string]string)
	for _, r := range rows {
		rowMap[r.StringPath("/")] = r.Value
	}
	assert.Equal(t, "test", rowMap["name"])
	assert.Equal(t, "42", rowMap["id"])
	assert.Equal(t, "test@example.com", rowMap["email"])
}

// TestProtobuf_SchemaFromFile tests schema-aware decoding using .proto and
// .pb files on disk.
func TestProtobuf_SchemaFromFile(t *testing.T) {
	t.Parallel()

	protoFile := filepath.Join("testdata", "simple.proto")
	pbFile := filepath.Join("testdata", "simple.pb")

	protoSource, err := os.ReadFile(protoFile)
	require.NoError(t, err)

	pbData, err := os.ReadFile(pbFile)
	require.NoError(t, err)

	schemaFn := ProtobufWithSchema(protoSource, "Simple")
	rows, err := schemaFn(pbData)
	require.NoError(t, err)
	require.Len(t, rows, 3)

	rowMap := make(map[string]string)
	for _, r := range rows {
		rowMap[r.StringPath("/")] = r.Value
	}
	assert.Equal(t, "test", rowMap["name"])
	assert.Equal(t, "42", rowMap["id"])
	assert.Equal(t, "test@example.com", rowMap["email"])
}

// TestProtobuf_SchemaWithQuery tests that query filtering works with
// schema-aware decoding.
func TestProtobuf_SchemaWithQuery(t *testing.T) {
	t.Parallel()

	schemaFn := ProtobufWithSchema([]byte(simpleProtoSchema), "Simple")
	rows, err := schemaFn(simplePBData, WithQuery([]string{"email"}))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "test@example.com", rows[0].Value)
	assert.Equal(t, []string{"email"}, rows[0].Path)
}

// TestProtobuf_SchemaMessageNotFound verifies a clear error when the
// message type doesn't exist in the schema.
func TestProtobuf_SchemaMessageNotFound(t *testing.T) {
	t.Parallel()

	schemaFn := ProtobufWithSchema([]byte(simpleProtoSchema), "DoesNotExist")
	_, err := schemaFn(simplePBData)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DoesNotExist")
}

// TestProtobuf_NestedSchemaless decodes a Person message (with nested Address
// and repeated tags) without a schema, verifying field-number-based paths.
func TestProtobuf_NestedSchemaless(t *testing.T) {
	t.Parallel()

	rows, err := Protobuf(personPBData)
	require.NoError(t, err)

	rowMap := make(map[string]string)
	for _, r := range rows {
		rowMap[r.StringPath("/")] = r.Value
	}

	assert.Equal(t, "Alice", rowMap["1"])
	assert.Equal(t, "1", rowMap["2"])
	assert.Equal(t, "123 Main St", rowMap["3/1"])
	assert.Equal(t, "Springfield", rowMap["3/2"])
	assert.Equal(t, "IL", rowMap["3/3"])
	assert.Equal(t, "admin", rowMap["4/0"])
	assert.Equal(t, "user", rowMap["4/1"])
}

// TestProtobuf_NestedSchemaAware decodes the same Person message with a
// schema, verifying field-name-based paths including nested messages.
func TestProtobuf_NestedSchemaAware(t *testing.T) {
	t.Parallel()

	schemaFn := ProtobufWithSchema([]byte(simpleProtoSchema), "Person")
	rows, err := schemaFn(personPBData)
	require.NoError(t, err)

	rowMap := make(map[string]string)
	for _, r := range rows {
		rowMap[r.StringPath("/")] = r.Value
	}

	assert.Equal(t, "Alice", rowMap["name"])
	assert.Equal(t, "1", rowMap["id"])
	assert.Equal(t, "123 Main St", rowMap["address/street"])
	assert.Equal(t, "Springfield", rowMap["address/city"])
	assert.Equal(t, "IL", rowMap["address/state"])
	assert.Equal(t, "admin", rowMap["tags/0"])
	assert.Equal(t, "user", rowMap["tags/1"])
}

// TestProtobuf_NestedSchemaAwareQuery verifies query filtering works with
// nested schema-aware decoding.
func TestProtobuf_NestedSchemaAwareQuery(t *testing.T) {
	t.Parallel()

	schemaFn := ProtobufWithSchema([]byte(simpleProtoSchema), "Person")
	rows, err := schemaFn(personPBData, WithQuery([]string{"address", "city"}))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "Springfield", rows[0].Value)
	assert.Equal(t, []string{"address", "city"}, rows[0].Path)
}

func sortRows(rows []Row) {
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].StringPath("/") < rows[j].StringPath("/")
	})
}
