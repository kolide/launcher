package indexeddb

import (
	"context"
	"testing"

	"github.com/kolide/launcher/pkg/log/multislogger"
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
		0xff, // version tag (indicates end of header)
		0x0f, // version
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

	obj, err := DeserializeChrome(context.TODO(), multislogger.NewNopLogger(), map[string][]byte{"data": testBytes})
	require.NoError(t, err, "deserializing object")

	// Confirm we got an id property for the object
	require.Contains(t, obj, "id", "expected id property")
}

func Test_deserializeIndexeddbValue_InvalidType(t *testing.T) {
	t.Parallel()

	testBytes := []byte{
		// header
		0x04, // padding, ignore
		0xff, // version tag
		0x01, // version
		0x00, // padding, ignore
		0x00, // padding, ignore
		0x00, // padding, ignore
		0xff, // version tag (indicates end of header)
		0x0f, // version
		// object
		0x6f, // object begin
		0x54, // boolean true tag -- invalid data!
		0x7b, // object end
		0x00, // properties_written
	}

	_, err := DeserializeChrome(context.TODO(), multislogger.NewNopLogger(), map[string][]byte{"data": testBytes})
	require.Error(t, err, "should not have been able to deserialize malformed object")
}
