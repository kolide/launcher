// Copyright (c) 2021-2025 cions
// Licensed under the MIT License.
//
// Package indexdbcomparator provides an implementation of the chrome indexeddb comparator, idb_cmp1. This contains
// the logic for comparing indexeddb keys so they can be properly iterated. idb_cmp1 is a more complex comparison mechanism
// than the default leveldb bytewiseComparator, taking into account key data types before determining
// how to proceed with comparison.
//
// This logic is largely taken from here:
// https://github.com/cions/leveldb-cli/blob/51a98cc00ca40e3eab4c96737938782909b0d644/indexeddb/comparer.go
// The logic is pulled in here and modified with a few changes:
// - wiring in our slogger to make debugging easier. the existing implementation relied on a recovered panic logging to stderr
// - removing panics in favor of passing more informational errors for debugging purposes
// - adds some additional notes and documentation
// - adds some unit tests
// We cannot change the comparator Compare signature (only returns an int), so in all cases where the implementation would have hit a panic
// and recovered we return the default value for int (0) after logging. This is functionally identical to the original implementation but should
// make any required investigations a bit easier, and ensure we ship logs for any errors we encounter.
//
// For additional reading:
// https://chromium.googlesource.com/chromium/src/+/main/content/browser/indexed_db/docs/leveldb_coding_scheme.md
// https://source.chromium.org/chromium/chromium/src/+/main:content/browser/indexed_db/indexed_db_leveldb_coding.cc

package indexeddbcomparator

import (
	"bytes"
	"cmp"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"math"

	"github.com/kolide/goleveldb/leveldb/comparer"
)

// const values are defined here: https://source.chromium.org/chromium/chromium/src/+/main:content/browser/indexed_db/indexed_db_leveldb_coding.cc
const (
	globalMetadata   = 0
	databaseMetadata = 1
	objectStoreData  = 2
	existsEntry      = 3
	indexData        = 4
	invalidType      = 5
	blobEntry        = 6
)

const (
	objectStoreDataIndexId = 1
	existsEntryIndexId     = 2
	blobEntryIndexId       = 3
	minimumIndexId         = 30
)

const (
	maxSimpleGlobalMetaDataTypeByte = 7
	scopesPrefixByte                = 50
	databaseFreeListTypeByte        = 100
	databaseNameTypeByte            = 201
)

const (
	maxSimpleDatabaseMetaDataTypeByte = 6
	objectStoreMetaDataTypeByte       = 50
	indexMetaDataTypeByte             = 100
	objectStoreFreeListTypeByte       = 150
	indexFreeListTypeByte             = 151
	objectStoreNamesTypeByte          = 200
	indexNamesKeyTypeByte             = 201
)

const (
	indexedDBKeyNullTypeByte   = 0
	indexedDBKeyStringTypeByte = 1
	indexedDBKeyDateTypeByte   = 2
	indexedDBKeyNumberTypeByte = 3
	indexedDBKeyArrayTypeByte  = 4
	indexedDBKeyMinKeyTypeByte = 5
	indexedDBKeyBinaryTypeByte = 6
)

const (
	indexedDBInvalidKeyType = 0
	indexedDBArrayKeyType   = 1
	indexedDBBinaryKeyType  = 2
	indexedDBStringKeyType  = 3
	indexedDBDateKeyType    = 4
	indexedDBNumberKeyType  = 5
	indexedDBNoneKeyType    = 6
	indexedDBMinKeyType     = 7
)

type (
	idbCmp1Comparer struct {
		slogger *slog.Logger
	}

	keyPrefix struct {
		DatabaseId, ObjectStoreId, IndexId int64
	}
)

func NewIdbCmp1Comparer(slogger *slog.Logger) comparer.Comparer {
	return &idbCmp1Comparer{
		slogger: slogger.With("component", "idb_cmp1_comparer"),
	}
}

func (prefix *keyPrefix) Type() int {
	switch {
	case prefix.DatabaseId == 0:
		return globalMetadata
	case prefix.ObjectStoreId == 0:
		return databaseMetadata
	case prefix.IndexId == objectStoreDataIndexId:
		return objectStoreData
	case prefix.IndexId == existsEntryIndexId:
		return existsEntry
	case prefix.IndexId == blobEntryIndexId:
		return blobEntry
	case prefix.IndexId >= minimumIndexId:
		return indexData
	default:
		return invalidType
	}
}

func (c *idbCmp1Comparer) Compare(a, b []byte) int {
	slogger := c.slogger.With(
		"key_a", fmt.Sprintf("%x", a),
		"key_b", fmt.Sprintf("%x", b),
	)

	aWithoutPrefix, prefixA, err := decodeKeyPrefix(a)
	if err != nil {
		slogger.Log(context.TODO(), slog.LevelError,
			"error decoding key prefix a",
			"err", err,
		)
		return 0
	}

	bWithoutPrefix, prefixB, err := decodeKeyPrefix(b)
	if err != nil {
		slogger.Log(context.TODO(), slog.LevelError,
			"error decoding key prefix b",
			"err", err,
		)
		return 0
	}

	// Compare the key prefixes. this should be the end of the comparison for any keys which belong to
	// a separate database, object store, or index
	if ret := compareKeyPrefix(prefixA, prefixB); ret != 0 {
		return ret
	}

	// if we got here, we know we're looking at two keys which share identical prefixes. now we need to compare
	// the key data itself, and these comparisons are performed based on the type of the key prefixes being compared.
	switch prefixA.Type() {
	case globalMetadata:
		if len(aWithoutPrefix) == 0 || len(bWithoutPrefix) == 0 {
			return cmp.Compare(len(aWithoutPrefix), len(bWithoutPrefix))
		}
		if ret := cmp.Compare(aWithoutPrefix[0], bWithoutPrefix[0]); ret != 0 {
			return ret
		}

		typeByte := aWithoutPrefix[0]
		aWithoutPrefix, bWithoutPrefix = aWithoutPrefix[1:], bWithoutPrefix[1:]

		// this flow seems kind of odd but it does align with the chromium implementation here:
		// https://chromium.googlesource.com/chromium/src/+/main/content/browser/indexed_db/indexed_db_leveldb_coding.cc#1365.
		// best guess is that by this point we already know that key prefixes are equal, and that the type byte is equal,
		// so maybe simple global metadata types are just considered equal for comparison purposes and there'd be no reason to go further here.
		if typeByte < maxSimpleGlobalMetaDataTypeByte {
			return 0
		}

		switch typeByte {
		case scopesPrefixByte:
			return bytes.Compare(aWithoutPrefix, bWithoutPrefix)
		case databaseFreeListTypeByte:
			if len(aWithoutPrefix) == 0 || len(bWithoutPrefix) == 0 {
				return cmp.Compare(len(aWithoutPrefix), len(bWithoutPrefix))
			}
			_, databaseIdA, err := c.decodeVarInt(aWithoutPrefix)
			if err != nil {
				return 0 // error already logged by decodeVarInt
			}
			_, databaseIdB, err := c.decodeVarInt(bWithoutPrefix)
			if err != nil {
				return 0 // error already logged by decodeVarInt
			}
			return cmp.Compare(databaseIdA, databaseIdB)
		case databaseNameTypeByte:
			if len(aWithoutPrefix) == 0 || len(bWithoutPrefix) == 0 {
				return cmp.Compare(len(aWithoutPrefix), len(bWithoutPrefix))
			}
			aPostString, bPostString, ret, err := c.compareStringWithLength(aWithoutPrefix, bWithoutPrefix)
			if err != nil {
				return 0 // error already logged by compareStringWithLength via decodeVarInt
			}
			if ret != 0 {
				return ret
			}

			// if we got here we know that compareStringWithLength has determined a and b to be equal for the first string lengths,
			// and a/bPostString contain the remaining bytes after the string data.
			// if either of these values is now empty, we make the comparison based on the length of the remaining bytes.
			// otherwise we'll fall through to another string comparison of the remaining key data
			if len(aPostString) == 0 || len(bPostString) == 0 {
				return cmp.Compare(len(aPostString), len(bPostString))
			}
			_, _, ret, err = c.compareStringWithLength(aPostString, bPostString)
			if err != nil {
				return 0 // error already logged by compareStringWithLength via decodeVarInt
			}
			return ret
		default:
			c.slogger.Log(context.TODO(), slog.LevelError,
				"invalid key prefix type byte for prefix a",
				"prefix", prefixA,
				"type_byte", fmt.Sprintf("%x", typeByte),
			)
			return 0
		}
	case databaseMetadata:
		if len(aWithoutPrefix) == 0 || len(bWithoutPrefix) == 0 {
			return cmp.Compare(len(aWithoutPrefix), len(bWithoutPrefix))
		}
		if ret := cmp.Compare(aWithoutPrefix[0], bWithoutPrefix[0]); ret != 0 {
			return ret
		}

		typeByte := aWithoutPrefix[0]
		aWithoutPrefix, bWithoutPrefix = aWithoutPrefix[1:], bWithoutPrefix[1:]

		if typeByte < maxSimpleDatabaseMetaDataTypeByte {
			return 0
		}

		switch typeByte {
		case objectStoreMetaDataTypeByte:
			if len(aWithoutPrefix) == 0 || len(bWithoutPrefix) == 0 {
				return cmp.Compare(len(aWithoutPrefix), len(bWithoutPrefix))
			}
			aPostObjectStoreId, objectStoreIdA, err := c.decodeVarInt(aWithoutPrefix)
			if err != nil {
				return 0 // error already logged by decodeVarInt
			}
			bPostObjectStoreId, objectStoreIdB, err := c.decodeVarInt(bWithoutPrefix)
			if err != nil {
				return 0 // error already logged by decodeVarInt
			}
			if ret := cmp.Compare(objectStoreIdA, objectStoreIdB); ret != 0 {
				return ret
			}

			// if we got here we know that we've pulled identical object store ids from both keys.
			// but we may have key data remaining- if either is empty, we can only compare the lengths.
			// otherwise we go on to compare the first byte of each.
			if len(aPostObjectStoreId) == 0 || len(bPostObjectStoreId) == 0 {
				return cmp.Compare(len(aPostObjectStoreId), len(bPostObjectStoreId))
			}
			return cmp.Compare(aPostObjectStoreId[0], bPostObjectStoreId[0])
		case indexMetaDataTypeByte:
			if len(aWithoutPrefix) == 0 || len(bWithoutPrefix) == 0 {
				return cmp.Compare(len(aWithoutPrefix), len(bWithoutPrefix))
			}
			a, objectStoreIdA, err := c.decodeVarInt(aWithoutPrefix)
			if err != nil {
				return 0 // error already logged by decodeVarInt
			}
			b, objectStoreIdB, err := c.decodeVarInt(bWithoutPrefix)
			if err != nil {
				return 0 // error already logged by decodeVarInt
			}
			if ret := cmp.Compare(objectStoreIdA, objectStoreIdB); ret != 0 {
				return ret
			}

			if len(a) == 0 || len(b) == 0 {
				return cmp.Compare(len(a), len(b))
			}
			a, indexIdA, err := c.decodeVarInt(a)
			if err != nil {
				return 0 // error already logged by decodeVarInt
			}
			b, indexIdB, err := c.decodeVarInt(b)
			if err != nil {
				return 0 // error already logged by decodeVarInt
			}
			if ret := cmp.Compare(indexIdA, indexIdB); ret != 0 {
				return ret
			}

			if len(a) == 0 || len(b) == 0 {
				return cmp.Compare(len(a), len(b))
			}
			return cmp.Compare(a[0], b[0])
		case objectStoreFreeListTypeByte:
			if len(aWithoutPrefix) == 0 || len(bWithoutPrefix) == 0 {
				return cmp.Compare(len(aWithoutPrefix), len(bWithoutPrefix))
			}
			_, objectStoreIdA, err := c.decodeVarInt(aWithoutPrefix)
			if err != nil {
				return 0 // error already logged by decodeVarInt
			}
			_, objectStoreIdB, err := c.decodeVarInt(bWithoutPrefix)
			if err != nil {
				return 0 // error already logged by decodeVarInt
			}
			return cmp.Compare(objectStoreIdA, objectStoreIdB)
		case indexFreeListTypeByte:
			if len(aWithoutPrefix) == 0 || len(bWithoutPrefix) == 0 {
				return cmp.Compare(len(aWithoutPrefix), len(bWithoutPrefix))
			}
			a, objectStoreIdA, err := c.decodeVarInt(aWithoutPrefix)
			if err != nil {
				return 0 // error already logged by decodeVarInt
			}
			b, objectStoreIdB, err := c.decodeVarInt(bWithoutPrefix)
			if err != nil {
				return 0 // error already logged by decodeVarInt
			}
			if ret := cmp.Compare(objectStoreIdA, objectStoreIdB); ret != 0 {
				return ret
			}

			if len(a) == 0 || len(b) == 0 {
				return cmp.Compare(len(a), len(b))
			}
			_, indexIdA, err := c.decodeVarInt(a)
			if err != nil {
				return 0 // error already logged by decodeVarInt
			}
			_, indexIdB, err := c.decodeVarInt(b)
			if err != nil {
				return 0 // error already logged by decodeVarInt
			}
			return cmp.Compare(indexIdA, indexIdB)
		case objectStoreNamesTypeByte:
			if len(aWithoutPrefix) == 0 || len(bWithoutPrefix) == 0 {
				return cmp.Compare(len(aWithoutPrefix), len(bWithoutPrefix))
			}
			_, _, ret, err := c.compareStringWithLength(aWithoutPrefix, bWithoutPrefix)
			if err != nil {
				return 0 // error already logged by compareStringWithLength via decodeVarInt
			}
			return ret
		case indexNamesKeyTypeByte:
			if len(aWithoutPrefix) == 0 || len(bWithoutPrefix) == 0 {
				return cmp.Compare(len(aWithoutPrefix), len(bWithoutPrefix))
			}
			a, objectStoreIdA, err := c.decodeVarInt(aWithoutPrefix)
			if err != nil {
				return 0 // error already logged by decodeVarInt
			}
			b, objectStoreIdB, err := c.decodeVarInt(bWithoutPrefix)
			if err != nil {
				return 0 // error already logged by decodeVarInt
			}
			if ret := cmp.Compare(objectStoreIdA, objectStoreIdB); ret != 0 {
				return ret
			}

			if len(a) == 0 || len(b) == 0 {
				return cmp.Compare(len(a), len(b))
			}
			_, _, ret, err := c.compareStringWithLength(a, b)
			if err != nil {
				return 0 // error already logged by compareStringWithLength via decodeVarInt
			}
			return ret
		default:
			c.slogger.Log(context.TODO(), slog.LevelError,
				"invalid key prefix type byte for databaseMetadata case",
				"type_byte", fmt.Sprintf("%x", typeByte),
			)
			return 0
		}
	case objectStoreData:
		_, _, ret, err := c.compareEncodedIDBKeys(aWithoutPrefix, bWithoutPrefix)
		if err != nil {
			c.slogger.Log(context.TODO(), slog.LevelError,
				"encountered error comparing encoded IDB keys",
				"err", err,
			)
			return 0
		}
		return ret
	case existsEntry:
		_, _, ret, err := c.compareEncodedIDBKeys(aWithoutPrefix, bWithoutPrefix)
		if err != nil {
			c.slogger.Log(context.TODO(), slog.LevelError,
				"encountered error comparing encoded IDB keys",
				"err", err,
			)
			return 0
		}
		return ret
	case blobEntry:
		_, _, ret, err := c.compareEncodedIDBKeys(aWithoutPrefix, bWithoutPrefix)
		if err != nil {
			c.slogger.Log(context.TODO(), slog.LevelError,
				"encountered error comparing encoded IDB keys",
				"err", err,
			)
			return 0
		}
		return ret
	case indexData:
		a, b, ret, err := c.compareEncodedIDBKeys(aWithoutPrefix, bWithoutPrefix)
		if err != nil {
			c.slogger.Log(context.TODO(), slog.LevelError,
				"encountered error comparing encoded IDB keys",
				"err", err,
			)
			return 0
		}
		if ret != 0 {
			return ret
		}

		sequenceNumberA := int64(-1)
		sequenceNumberB := int64(-1)
		if len(a) > 0 {
			a, sequenceNumberA, err = c.decodeVarInt(a)
			if err != nil {
				return 0 // error already logged by decodeVarInt
			}
		}
		if len(b) > 0 {
			b, sequenceNumberB, err = c.decodeVarInt(b)
			if err != nil {
				return 0 // error already logged by decodeVarInt
			}
		}

		if len(a) == 0 || len(b) == 0 {
			return cmp.Compare(len(a), len(b))
		}
		_, _, ret, err = c.compareEncodedIDBKeys(a, b)
		if err != nil {
			c.slogger.Log(context.TODO(), slog.LevelError,
				"encountered error comparing encoded IDB keys",
				"err", err,
			)
			return 0
		}
		if ret != 0 {
			return ret
		}

		return cmp.Compare(sequenceNumberA, sequenceNumberB)
	default:
		c.slogger.Log(context.TODO(), slog.LevelError,
			"invalid key prefix type",
			"prefix_type", prefixA.Type(),
		)
		return 0
	}
}

func (c *idbCmp1Comparer) Name() string {
	return "idb_cmp1"
}

func (c *idbCmp1Comparer) Separator(dst, a, b []byte) []byte {
	return nil
}

func (c *idbCmp1Comparer) Successor(dst, b []byte) []byte {
	return nil
}

// decodeVarInt decodes a variable-length integer from the beginning of a byte slice.
// It follows the IndexedDB LevelDB coding scheme where integers are encoded as
// variable-width, little-endian, 7 bits per byte with the high bit set until the last byte.
// Returns the remaining bytes after the decoded integer, the decoded int64 value, and any error.
// Returns an error if the input contains an invalid variable-length integer encoding.
// see https://chromium.googlesource.com/chromium/src/+/main/content/browser/indexed_db/docs/leveldb_coding_scheme.md#primitive-types
func (c *idbCmp1Comparer) decodeVarInt(a []byte) ([]byte, int64, error) {
	v := uint64(0)
	for i := 0; i < len(a) && i < 9; i++ {
		v |= uint64(a[i]&0x7f) << (7 * i)
		if a[i]&0x80 == 0 {
			return a[i+1:], int64(v), nil
		}
	}

	c.slogger.Log(context.TODO(), slog.LevelError,
		"invalid key provided for decodeVarInt",
		"invalid_key", fmt.Sprintf("%x", a),
	)

	return nil, 0, fmt.Errorf("invalid key provided for decodeVarInt: %x", a)
}

func (c *idbCmp1Comparer) compareBinary(a, b []byte) ([]byte, []byte, int, error) {
	a, lenA, err := c.decodeVarInt(a)
	if err != nil {
		return nil, nil, 0, err
	}

	b, lenB, err := c.decodeVarInt(b)
	if err != nil {
		return nil, nil, 0, err
	}

	if uint64(len(a)) < uint64(lenA) || uint64(len(b)) < uint64(lenB) {
		minlen := min(uint64(lenA), uint64(lenB), uint64(len(a)), uint64(len(b)))
		if ret := bytes.Compare(a[:minlen], b[:minlen]); ret != 0 {
			return nil, nil, ret, nil
		}
		return nil, nil, cmp.Compare(lenA, lenB), nil
	}

	return a[lenA:], b[lenB:], bytes.Compare(a[:lenA], b[:lenB]), nil
}

// compareDouble compares two encoded double values from IndexedDB keys.
// It extracts 8-byte double values from the beginning of each byte slice,
// converts them from their native endian representation to float64 values,
// and returns the remaining bytes along with the comparison result.
// Returns an error if either input slice is less than 8 bytes.
func (c *idbCmp1Comparer) compareDouble(a, b []byte) ([]byte, []byte, int, error) {
	if len(a) < 8 || len(b) < 8 {
		c.slogger.Log(context.TODO(), slog.LevelError,
			"invalid keys provided for compareDouble (must be at least 8 bytes)",
			"key_len_a", len(a),
			"key_len_b", len(b),
		)

		return nil, nil, 0, errors.New("invalid keys provided for compareDouble (must be at least 8 bytes)")
	}

	f1 := math.Float64frombits(binary.NativeEndian.Uint64(a))
	f2 := math.Float64frombits(binary.NativeEndian.Uint64(b))
	return a[8:], b[8:], cmp.Compare(f1, f2), nil
}

func (c *idbCmp1Comparer) compareEncodedIDBKeys(a, b []byte) ([]byte, []byte, int, error) {
	if len(a) == 0 || len(b) == 0 {
		return a, b, cmp.Compare(len(a), len(b)), nil
	}

	ret := cmp.Compare(keyTypeByteToKeyType(a[0]), keyTypeByteToKeyType(b[0]))
	if ret != 0 {
		return a[1:], b[1:], ret, nil
	}

	typeByte := a[0]
	a, b = a[1:], b[1:]

	switch typeByte {
	case indexedDBKeyNullTypeByte, indexedDBKeyMinKeyTypeByte:
		return a, b, 0, nil
	case indexedDBKeyArrayTypeByte:
		if len(a) == 0 || len(b) == 0 {
			return a, b, cmp.Compare(len(a), len(b)), nil
		}
		a, lenA, err := c.decodeVarInt(a)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("decoding key a (%x) as varInt: %w", a, err)
		}
		b, lenB, err := c.decodeVarInt(b)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("decoding key b (%x) as varInt: %w", b, err)
		}
		for range min(lenA, lenB) {
			if len(a) == 0 || len(b) == 0 {
				break
			}
			a, b, ret, err = c.compareEncodedIDBKeys(a, b)
			if err != nil {
				return nil, nil, 0, fmt.Errorf("recursively comparing encodedIdbKeys: %w", err)
			}
			if ret != 0 {
				return a, b, ret, nil
			}
		}
		return a, b, cmp.Compare(lenA, lenB), nil
	case indexedDBKeyBinaryTypeByte:
		if len(a) == 0 || len(b) == 0 {
			return a, b, cmp.Compare(len(a), len(b)), nil
		}
		return c.compareBinary(a, b)
	case indexedDBKeyStringTypeByte:
		if len(a) == 0 || len(b) == 0 {
			return a, b, cmp.Compare(len(a), len(b)), nil
		}
		return c.compareStringWithLength(a, b)
	case indexedDBKeyDateTypeByte, indexedDBKeyNumberTypeByte:
		if len(a) == 0 || len(b) == 0 {
			return a, b, cmp.Compare(len(a), len(b)), nil
		}
		return c.compareDouble(a, b)
	default:
		return nil, nil, 0, fmt.Errorf("invalid keyTypeByte provided for compareEncodedIDBKeys: %d", typeByte)
	}
}

// compareStringWithLength compares two IndexedDB string keys with their encoded lengths.
// It decodes the variable-length integer prefixes that specify the string lengths,
// then compares the encoded string data byte-by-byte.
// Returns the remaining bytes after the strings, and the comparison result.
// If either key has insufficient bytes for its declared length, compares available bytes
// and falls back to comparing the length values themselves.
func (c *idbCmp1Comparer) compareStringWithLength(a, b []byte) ([]byte, []byte, int, error) {
	a, v1, err := c.decodeVarInt(a)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("decoding key a (%x) as varInt: %w", a, err)
	}

	lenA := 2 * uint64(v1)
	b, v2, err := c.decodeVarInt(b)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("decoding key b (%x) as varInt: %w", b, err)
	}

	lenB := 2 * uint64(v2)
	if uint64(len(a)) < lenA || uint64(len(b)) < lenB {
		minlen := min(lenA, lenB, uint64(len(a)), uint64(len(b)))
		if ret := bytes.Compare(a[:minlen], b[:minlen]); ret != 0 {
			return nil, nil, ret, nil
		}
		return nil, nil, cmp.Compare(v1, v2), nil
	}

	return a[lenA:], b[lenB:], bytes.Compare(a[:lenA], b[:lenB]), nil
}

func compareKeyPrefix(a, b *keyPrefix) int {
	if ret := cmp.Compare(a.DatabaseId, b.DatabaseId); ret != 0 {
		return ret
	}
	if ret := cmp.Compare(a.ObjectStoreId, b.ObjectStoreId); ret != 0 {
		return ret
	}
	if ret := cmp.Compare(a.IndexId, b.IndexId); ret != 0 {
		return ret
	}
	return 0
}

// decodeInt - see https://chromium.googlesource.com/chromium/src/+/main/content/browser/indexed_db/docs/leveldb_coding_scheme.md#primitive-types
// int64_t >= 0; 8 bytes in little-endian order
func decodeInt(a []byte) (int64, error) {
	if len(a) == 0 || len(a) > 8 {
		return 0, fmt.Errorf("invalid byte length for decodeInt: len=%d", len(a))
	}
	v := uint64(0)
	for i, b := range a {
		v |= uint64(b) << (8 * i)
	}
	return int64(v), nil
}

// decodeKeyPrefix extracts and decodes the key prefix from an IndexedDB key byte slice.
// It parses the first byte to determine the lengths of database ID, object store ID, and index ID fields,
// then decodes each field and returns the remaining bytes along with the decoded keyPrefix struct.
// Returns an error if the input is empty or has insufficient length for the expected prefix structure.
func decodeKeyPrefix(a []byte) ([]byte, *keyPrefix, error) {
	if len(a) == 0 {
		return nil, nil, errors.New("invalid empty key provided to decodeKeyPrefix")
	}

	firstByte := a[0]
	a = a[1:]

	// see here for chromium implementation: https://chromium.googlesource.com/chromium/src/+/main/content/browser/indexed_db/indexed_db_leveldb_coding.cc#1775
	// and here for documentation: https://chromium.googlesource.com/chromium/src/+/main/content/browser/indexed_db/docs/leveldb_coding_scheme.md#key-prefix
	// the top 3 bits are the length of the database id - 1. so here bits 5-7 store a 3-bit length value, add 1 to get the byte count (from 1-8 bytes)
	databaseIdBytes := int((((firstByte >> 5) & 0x07) + 1))
	// the next 3 bits are the length of the object store id - 1. so here bits 2-4 again store a 3-bit length value, add 1 to get the total byte count
	objectStoreIdBytes := int(((firstByte >> 2) & 0x07) + 1)
	// the last 2 are the same story but for the index id - 1
	indexIdBytes := int((firstByte & 0x03) + 1)

	if len(a) < databaseIdBytes+objectStoreIdBytes+indexIdBytes {
		return nil, nil, fmt.Errorf(
			"invalid key provided to decodeKeyPrefix (insufficient length for prefix bytes): key length %d, expected at least %d + %d + %d",
			len(a),
			databaseIdBytes,
			objectStoreIdBytes,
			indexIdBytes,
		)
	}

	databaseId, err := decodeInt(a[:databaseIdBytes])
	if err != nil {
		return nil, nil, fmt.Errorf("decoding database ID as int: %w", err)
	}
	a = a[databaseIdBytes:]

	objectStoreId, err := decodeInt(a[:objectStoreIdBytes])
	if err != nil {
		return nil, nil, fmt.Errorf("decoding object store ID as int: %w", err)
	}
	a = a[objectStoreIdBytes:]

	indexId, err := decodeInt(a[:indexIdBytes])
	if err != nil {
		return nil, nil, fmt.Errorf("decoding index ID as int: %w", err)
	}
	a = a[indexIdBytes:]

	return a, &keyPrefix{databaseId, objectStoreId, indexId}, nil
}

func keyTypeByteToKeyType(b byte) int {
	switch b {
	case indexedDBKeyNullTypeByte:
		return indexedDBInvalidKeyType
	case indexedDBKeyArrayTypeByte:
		return indexedDBArrayKeyType
	case indexedDBKeyBinaryTypeByte:
		return indexedDBBinaryKeyType
	case indexedDBKeyStringTypeByte:
		return indexedDBStringKeyType
	case indexedDBKeyDateTypeByte:
		return indexedDBDateKeyType
	case indexedDBKeyNumberTypeByte:
		return indexedDBNumberKeyType
	case indexedDBKeyMinKeyTypeByte:
		return indexedDBMinKeyType
	default:
		return indexedDBInvalidKeyType
	}
}
