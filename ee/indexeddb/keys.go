package indexeddb

import (
	"encoding/binary"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/text/encoding/unicode"
)

// databaseIdKey returns a key for querying the global metadata for the given `dbName`,
// which will return its id.
func databaseIdKey(databaseLocation string, dbName string) ([]byte, error) {
	// Construct a key to query the global metadata.
	databaseNameKey := []byte{
		0x00,
		0x00,
		0x00,
		0x00, // documentation says there should be only 3 of these, but I keep seeing 4.
		0xc9, // 201
	}

	// Next, append origin. I don't know why I have to append @1 to the origin name.
	originBytes, err := stringWithLength(strings.TrimSuffix(filepath.Base(databaseLocation), ".indexeddb.leveldb") + "@1")
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
	// TODO RM: this is sufficient for small strings, but will be an issue soon
	strLenBytes := make([]byte, 1)
	binary.PutUvarint(strLenBytes, uint64(len(strBytesUtf16)/2))

	return append(strLenBytes, strBytesUtf16...), nil
}
