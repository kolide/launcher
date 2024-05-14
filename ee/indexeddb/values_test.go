package indexeddb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_deserializeIndexeddbValue(t *testing.T) {
	t.Parallel()

	testBytes := []byte{
		// header
		0x04, // padding, ignore
		0xff, // version tag
		0x01, // version
		0x00, // padding, ignore
		0x00, // padding, ignore
		0x00, // padding, ignore
		// object
		0x6f, // object begin
		0x22, // string tag
		0x02, // string length of 2
		0x69, // i
		0x64, // d
		0x49, // int32 tag
		0x02, // value for ID
		0x7b, // object end
		0x01, // properties_written
	}

	obj, err := deserializeIndexeddbValue(testBytes)
	require.NoError(t, err, "deserializing object")

	// Confirm we got a version and data top-level property
	require.Contains(t, obj, "version", "expected version property")
	require.Contains(t, obj, "data", "expected data property")
	// Confirm we got an id property for the object
	require.Contains(t, obj["data"], "id", "expected id property")
}
