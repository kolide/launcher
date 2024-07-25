package katc

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"testing"

	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

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
