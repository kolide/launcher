package katc

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func Test_deserializeFirefox(t *testing.T) {
	t.Parallel()

	// Build expected object
	u, err := uuid.NewRandom()
	require.NoError(t, err, "generating test UUID")
	idValue := u.String()
	arrWithNestedObj := []string{"{\"id\":\"3\"}"}
	nestedArrBytes, err := json.Marshal(arrWithNestedObj)
	require.NoError(t, err)
	expectedObj := map[string][]byte{
		"id":      []byte(idValue), // will exercise deserializeString
		"version": []byte("1"),     // will exercise int deserialization
		"option":  nil,             // will exercise null/undefined deserialization
		"types":   nestedArrBytes,  // will exercise deserializeArray, deserializeNestedObject
	}

	// Build a serialized object to deserialize
	serializedObj := []byte{
		// Header
		0x00, 0x00, 0x00, 0x00, // header tag data -- discarded
		0x00, 0x00, 0xf1, 0xff, // LE `tagHeader`
		// Begin object
		0x00, 0x00, 0x00, 0x00, // object tag data -- discarded
		0x08, 0x00, 0xff, 0xff, // LE `tagObject`
		// Begin `id` key
		0x02, 0x00, 0x00, 0x80, // LE data about upcoming string: length 2 (remaining bytes), is ASCII
		0x04, 0x00, 0xff, 0xff, // LE `tagString`
		0x69, 0x64, // "id"
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // padding to get to 8-byte word boundary
		// End `id` key
		// Begin `id` value
		0x24, 0x00, 0x00, 0x80, // LE data about upcoming string: length 36 (remaining bytes), is ASCII
		0x04, 0x00, 0xff, 0xff, // LE `tagString`
	}
	// Append `id`
	serializedObj = append(serializedObj, []byte(idValue)...)
	// Append `id` padding, add `version`
	serializedObj = append(serializedObj,
		0x00, 0x00, 0x00, 0x00, // padding to get to 8-byte word boundary for `id` string
		// End `id` value
		// Begin `version` key
		0x07, 0x00, 0x00, 0x80, // LE data about upcoming string: length 7 (remaining bytes), is ASCII
		0x04, 0x00, 0xff, 0xff, // LE `tagString`
		0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, // "version"
		0x00, // padding to get to 8-byte word boundary
		// End `version` key
		// Begin `version` value
		0x01, 0x00, 0x00, 0x00, // Value `1`
		0x03, 0x00, 0xff, 0xff, // LE `tagInt32`
		// End `version` value
		// Begin `option` key
		0x06, 0x00, 0x00, 0x80, // LE data about upcoming string: length 6 (remaining bytes), is ASCII
		0x04, 0x00, 0xff, 0xff, // LE `tagString`
		0x6f, 0x70, 0x74, 0x69, 0x6f, 0x6e, // "option"
		0x00, 0x00, // padding to get to 8-byte word boundary
		// End `option` key
		// Begin `option` value
		0x00, 0x00, 0x00, 0x00, // Unused data, discarded
		0x00, 0x00, 0xff, 0xff, // LE `tagNull`
		// End `option` value
		// Begin `types` key
		0x05, 0x00, 0x00, 0x80, // LE data about upcoming string: length 5 (remaining bytes), is ASCII
		0x04, 0x00, 0xff, 0xff, // LE `tagString`
		0x74, 0x79, 0x70, 0x65, 0x73, // "types"
		0x00, 0x00, 0x00, // padding to get to 8-byte word boundary
		// End `types` key
		// Begin `types` value
		0x01, 0x00, 0x00, 0x00, // Array length (1)
		0x07, 0x00, 0xff, 0xff, // LE `tagArrayObject`
		// Begin first array item
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // An extra pair that gets discarded, I don't know why
		0x00, 0x00, 0x00, 0x00, // Tag data, discarded
		0x08, 0x00, 0xff, 0xff, // LE `tagObjectObject`
		// Begin nested object
		// Begin `id` key
		0x02, 0x00, 0x00, 0x80, // LE data about upcoming string: length 2 (remaining bytes), is ASCII
		0x04, 0x00, 0xff, 0xff, // LE `tagString`
		0x69, 0x64, // "id"
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // padding to get to 8-byte word boundary
		// End `id` key
		// Begin `id` value
		0x03, 0x00, 0x00, 0x00, // Value `3`
		0x03, 0x00, 0xff, 0xff, // LE `tagInt32`
		// End `id` value
		// Object footer
		0x00, 0x00, 0x00, 0x00, // tag data -- discarded
		0x13, 0x00, 0xff, 0xff, // LE `tagEndOfKeys` 0xffff0013
		// End nested object
		// End first array item
		// End `types` value
		// Object footer
		0x00, 0x00, 0x00, 0x00, // tag data -- discarded
		0x13, 0x00, 0xff, 0xff, // LE `tagEndOfKeys`
	)

	results, err := deserializeFirefox(context.TODO(), multislogger.NewNopLogger(), map[string][]byte{
		"data": serializedObj,
	})
	require.NoError(t, err, "expected to be able to deserialize object")

	require.Equal(t, expectedObj, results)
}

func Test_deserializeFirefox_missingTopLevelDataKey(t *testing.T) {
	t.Parallel()

	_, err := deserializeFirefox(context.TODO(), multislogger.NewNopLogger(), map[string][]byte{
		"not_a_data_key": nil,
	})
	require.Error(t, err, "expect deserializeFirefox requires top-level data key")
}

func Test_deserializeFirefox_malformedData(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName string
		data         []byte
	}{
		{
			testCaseName: "missing header",
			data: []byte{
				0x00, 0x00, 0x00, 0x00, // header tag data -- discarded
				0x00, 0x00, 0xff, 0xff, // LE `tagNull` (`tagHeader` expected instead)
			},
		},
		{
			testCaseName: "missing top-level object",
			data: []byte{
				// Header
				0x00, 0x00, 0x00, 0x00, // header tag data -- discarded
				0x00, 0x00, 0xf1, 0xff, // LE `tagHeader`
				// End header
				0x00, 0x00, 0x00, 0x00, // data about tag, not used
				0x04, 0x00, 0xff, 0xff, // LE `tagString` (`tagObject` expected instead)
			},
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			_, err := deserializeFirefox(context.TODO(), multislogger.NewNopLogger(), map[string][]byte{
				"data": tt.data,
			})
			require.Error(t, err, "expect deserializeFirefox rejects malformed data")
		})
	}
}

// Test_deserializeString tests that deserializeString can handle both ASCII and UTF-16 strings
func Test_deserializeString(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName string
		expected     []byte
		stringData   []byte
		stringBytes  []byte
	}{
		{
			testCaseName: "ascii",
			expected:     []byte("createdAt"),
			stringData: []byte{
				0x09, 0x00, 0x00, 0x80, // LE data about upcoming string: length 9 (remaining bytes), is ASCII (true)
			},
			stringBytes: []byte{
				0x63,                                     // c
				0x72,                                     // r
				0x65,                                     // e
				0x61,                                     // a
				0x74,                                     // t
				0x65,                                     // e
				0x64,                                     // d
				0x41,                                     // A
				0x74,                                     // t
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // padding to get to 8-byte word boundary
			},
		},
		{
			testCaseName: "utf-16",
			expected:     []byte("🏆"),
			stringData: []byte{
				0x02, 0x00, 0x00, 0x00, // LE data about upcoming string: length 2 (remaining bytes), is ASCII (false)
			},
			stringBytes: []byte{
				0x3c, 0xd8, 0xc6, 0xdf, // emoji: UTF-16 LE
				0x00, 0x00, 0x00, 0x00, // padding to get to 8-byte word boundary
			},
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			stringDataInt := binary.LittleEndian.Uint32(tt.stringData)
			stringReader := bytes.NewReader(tt.stringBytes)

			resultBytes, err := deserializeString(stringDataInt, stringReader)
			require.NoError(t, err)

			require.Equal(t, tt.expected, resultBytes)

			// Confirm we read all the padding in as well
			_, err = stringReader.ReadByte()
			require.Error(t, err)
			require.ErrorIs(t, err, io.EOF)
		})
	}
}

func Test_bitMask(t *testing.T) {
	t.Parallel()

	var expected uint32 = 0b01111111111111111111111111111111
	require.Equal(t, expected, bitMask(31))
}
