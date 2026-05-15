package indexeddb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

const (
	// See: https://github.com/chromium/chromium/blob/main/content/browser/indexed_db/indexed_db_leveldb_coding.cc
	// Types
	databaseNameTypeByte = 0xc9 // 201

	// Metadata types
	objectStoreMetaDataTypeByte = 0x32 // 50

	// Index IDs
	objectStoreDataIndexId = 0x01 // 1
	blobEntryIndexId       = 0x03 // 3

	// When parsing the origin from the database location, I have to add @1 at the end for the origin to be complete.
	// I don't know why.
	originSuffix = "@1"
)

// databaseIdKey returns a key for querying the global metadata for the given `dbName`,
// which will return its id.
func databaseIdKey(databaseLocation string, dbName string) ([]byte, error) {
	// Construct a key to query the global metadata.
	databaseNameKey := []byte{
		0x00,
		0x00,
		0x00,
		0x00,                 // documentation says there should be only 3 of these, but I keep seeing 4.
		databaseNameTypeByte, // 201
	}

	// Next, append origin.
	originBytes, err := stringWithLength(strings.TrimSuffix(filepath.Base(databaseLocation), ".indexeddb.leveldb") + originSuffix)
	if err != nil {
		return nil, fmt.Errorf("constructing StringWithLength: %w", err)
	}
	databaseNameKey = append(databaseNameKey, originBytes...)

	// Now, the same thing, but for the database name.
	dbNameBytes, err := stringWithLength(dbName)
	if err != nil {
		return nil, fmt.Errorf("constructing StringWithLength: %w", err)
	}
	databaseNameKey = append(databaseNameKey, dbNameBytes...)

	return databaseNameKey, nil
}

// objectStoreNameKey constructs a query for the name for the object store with the given ID.
func objectStoreNameKey(dbId uint64, objectStoreId uint64) []byte {
	// Key takes the format <0, database id, 0, 0, 50, object store id, 0>.
	storeNameKey := []byte{0x00}
	storeNameKey = append(storeNameKey, uvarintToBytes(dbId)...)
	storeNameKey = append(storeNameKey,
		0x00,
		0x00,
		objectStoreMetaDataTypeByte,
	)

	// Add the object store ID
	storeNameKey = append(storeNameKey, uvarintToBytes(objectStoreId)...)

	// Add 0x00, indicating we're querying for the object store name
	return append(storeNameKey, 0x00)
}

// objectDataKeyPrefix returns the key prefix shared by all objects stored in the given database
// and in the given store.
func objectDataKeyPrefix(dbId uint64, objectStoreId uint64) []byte {
	keyPrefix := []byte{0x00}
	keyPrefix = append(keyPrefix, uvarintToBytes(dbId)...)
	keyPrefix = append(keyPrefix, uvarintToBytes(objectStoreId)...)

	return append(keyPrefix, objectStoreDataIndexId)
}

// blobKeyPrefix returns the key prefix for all ExternalObjects (blobs, files, and file system access handles)
// for the given database and object store.
func blobKeyPrefix(dbId uint64, objectStoreId uint64) []byte {
	keyPrefix := []byte{0x00}
	keyPrefix = append(keyPrefix, uvarintToBytes(dbId)...)
	keyPrefix = append(keyPrefix, uvarintToBytes(objectStoreId)...)

	return append(keyPrefix, blobEntryIndexId)
}

func decodeUtf16BigEndianBytes(b []byte) ([]byte, error) {
	utf16BigEndianDecoder := unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM).NewDecoder()
	return utf16BigEndianDecoder.Bytes(b)
}

// readStringWithLength reads the upcoming StringWithLength from the byte reader.
// It takes the following format:
// VarInt prefix with length in code units (i.e. bytes/2), followed by String (UTF-16 BE)
func readStringWithLength(valueReader *bytes.Reader) (string, error) {
	stringLengthInCodeUnits, err := binary.ReadUvarint(valueReader)
	if err != nil {
		return "", fmt.Errorf("reading string length: %w", err)
	}

	stringLength := stringLengthInCodeUnits * 2
	if stringLength > uint64(valueReader.Len()) {
		return "", fmt.Errorf("cannot read StringWithLength: length %d but only %d bytes remaining to read", stringLength, valueReader.Len())
	}

	rawString := make([]byte, stringLength)
	for i := 0; i < int(stringLength); i++ {
		nextByte, err := valueReader.ReadByte()
		if err != nil {
			return "", fmt.Errorf("reading byte at index %d in string of length %d: %w", i, stringLength, err)
		}
		rawString[i] = nextByte
	}

	// Strings are UTF-16 BE
	decoded, _, err := transform.Bytes(unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM).NewDecoder(), rawString)
	if err != nil {
		return "", fmt.Errorf("reading string as utf-16: %w", err)
	}

	return string(decoded), nil
}

// stringWithLength constructs an appropriate representation of `s`.
// See: https://github.com/chromium/chromium/blob/main/content/browser/indexed_db/docs/leveldb_coding_scheme.md#types
func stringWithLength(s string) ([]byte, error) {
	// Strings are UTF-16 BE -- prepare an encoder to encode them
	utf16BigEndianEncoder := unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM).NewEncoder()

	// Construct a StringWithLength: first, the length of the string in code units (i.e. bytes/2),
	// then the string itself.
	// The int is int64_t >= 0; variable-width, little-endian.
	strBytes := []byte(s)
	strBytesUtf16, err := utf16BigEndianEncoder.Bytes(strBytes)
	if err != nil {
		return nil, fmt.Errorf("encoding string as utf-16: %w", err)
	}
	strLenBytes := uvarintToBytes(uint64(len(strBytesUtf16) / 2))

	return append(strLenBytes, strBytesUtf16...), nil
}

func uvarintToBytes(x uint64) []byte {
	buf := make([]byte, binary.MaxVarintLen64)
	bytesWritten := binary.PutUvarint(buf, x)

	return buf[:bytesWritten]
}
