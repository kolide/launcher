package indexeddb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/text/encoding/unicode"
)

func Test_databaseIdKey(t *testing.T) {
	t.Parallel()

	testOrigin := "chrome-extension_testtesttest_3"
	testDatabasePath := filepath.Join("some", "path", "to", fmt.Sprintf("%s.indexeddb.leveldb", testOrigin))
	testDatabaseName := "example-db"

	testNameKey, err := databaseIdKey(testDatabasePath, testDatabaseName)
	require.NoError(t, err)

	// Validate prefix
	require.True(t, bytes.HasPrefix(testNameKey, []byte{0x00, 0x00, 0x00, 0x00, databaseNameTypeByte}), "key does not have expected prefix")

	// Confirm database origin and name are both in the key somewhere
	testOriginBytes, err := stringWithLength(testOrigin + originSuffix)
	require.NoError(t, err, "getting origin bytes")
	require.True(t, bytes.Contains(testNameKey, testOriginBytes), "origin missing from key")
	testDatabaseNameBytes, err := stringWithLength(testDatabaseName)
	require.NoError(t, err, "getting database name bytes")
	require.True(t, bytes.Contains(testNameKey, testDatabaseNameBytes), "database name missing from key")
}

func Test_objectStoreNameKey(t *testing.T) {
	t.Parallel()

	var dbId uint64 = 2
	var objectStoreId uint64 = 3

	// Key takes the format <0, database id, 0, 0, 50, object store id, 0>.
	expectedKey := []byte{
		0x00,
		0x02, // DB ID
		0x00,
		0x00,
		objectStoreMetaDataTypeByte,
		0x03, // object store ID
		0x00,
	}

	require.Equal(t, expectedKey, objectStoreNameKey(dbId, objectStoreId), "object store name key format is incorrect")
}

func Test_objectDataKeyPrefix(t *testing.T) {
	t.Parallel()

	var dbId uint64 = 4
	var objectStoreId uint64 = 1

	expectedKeyPrefix := []byte{
		0x00,
		0x04,                   // DB ID
		0x01,                   // object store ID
		objectStoreDataIndexId, // the index indicating we want the stored object data
	}

	require.Equal(t, expectedKeyPrefix, objectDataKeyPrefix(dbId, objectStoreId), "key prefix format is incorrect")
}

func Test_decodeUtf16BigEndianBytes(t *testing.T) {
	t.Parallel()

	originalBytes := []byte("testing-testing")
	utf16BigEndianEncoder := unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM).NewEncoder()
	utf16Bytes, err := utf16BigEndianEncoder.Bytes(originalBytes)
	require.NoError(t, err, "encoding bytes")

	actualBytes, err := decodeUtf16BigEndianBytes(utf16Bytes)
	require.NoError(t, err, "decoding bytes")
	require.Equal(t, originalBytes, actualBytes, "decoded bytes do not match")
}

func Test_stringWithLength(t *testing.T) {
	t.Parallel()

	testStr := "testing-string"
	_, err := stringWithLength(testStr)
	require.NoError(t, err)
}

func Test_uvarintToBytes(t *testing.T) {
	t.Parallel()

	for _, i := range []uint64{
		0, // min uint64 value
		35,
		128,
		18446744073709551615, // max uint64 value
	} {
		// convert int to bytes
		intBytes := uvarintToBytes(i)

		// now go bytes => int and confirm we get the same testInt we started with
		convertedInt, _ := binary.Uvarint(intBytes)
		require.Equal(t, i, convertedInt)
	}
}
