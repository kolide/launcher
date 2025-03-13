package katc

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"time"

	"github.com/kolide/launcher/pkg/traces"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

const (
	tagFloatMax     uint32 = 0xfff00000
	tagHeader       uint32 = 0xfff10000
	tagNull         uint32 = 0xffff0000
	tagUndefined    uint32 = 0xffff0001
	tagBoolean      uint32 = 0xffff0002
	tagInt32        uint32 = 0xffff0003
	tagString       uint32 = 0xffff0004
	tagDateObject   uint32 = 0xffff0005
	tagRegexpObject uint32 = 0xffff0006
	tagArrayObject  uint32 = 0xffff0007
	tagObjectObject uint32 = 0xffff0008
	// SCTAG_ARRAY_BUFFER_OBJECT_V2 omitted
	tagBooleanObject    uint32 = 0xffff000a
	tagStringObject     uint32 = 0xffff000b
	tagNumberObject     uint32 = 0xffff000c
	tagBackReferenceObj uint32 = 0xffff000d
	// SCTAG_DO_NOT_USE_1 omitted
	// SCTAG_DO_NOT_USE_2 omitted
	// SCTAG_TYPED_ARRAY_OBJECT_V2 omitted
	tagMapObject uint32 = 0xffff0011
	tagSetObject uint32 = 0xffff0012
	tagEndOfKeys uint32 = 0xffff0013
	// SCTAG_DO_NOT_USE_3 omitted
	// SCTAG_DATA_VIEW_OBJECT_V2 omitted
	// SCTAG_SAVED_FRAME_OBJECT omitted
	// SCTAG_JSPRINCIPALS omitted
	// SCTAG_NULL_JSPRINCIPALS omitted
	// SCTAG_RECONSTRUCTED_SAVED_FRAME_PRINCIPALS_IS_SYSTEM omitted
	// SCTAG_RECONSTRUCTED_SAVED_FRAME_PRINCIPALS_IS_NOT_SYSTEM omitted
	// SCTAG_SHARED_ARRAY_BUFFER_OBJECT omitted
	// SCTAG_SHARED_WASM_MEMORY_OBJECT omitted
	tagBigInt                       uint32 = 0xffff001d
	tagBigIntObject                 uint32 = 0xffff001e
	tagArrayBufferObj               uint32 = 0xffff001f
	tagTypedArrayObj                uint32 = 0xffff0020
	tagDataViewObj                  uint32 = 0xffff0021
	tagErrorObj                     uint32 = 0xffff0022
	tagResizableArrayBufferObj      uint32 = 0xffff0023
	tagGrowableSharedArrayBufferObj uint32 = 0xffff0024
)

// deserializeFirefox deserializes a JS object that has been stored by Firefox
// in IndexedDB sqlite-backed databases.
// References:
// * https://stackoverflow.com/a/59923297
// * https://searchfox.org/mozilla-central/source/js/src/vm/StructuredClone.cpp (see especially JSStructuredCloneReader::read)
func deserializeFirefox(ctx context.Context, slogger *slog.Logger, row map[string][]byte) (map[string][]byte, error) {
	_, span := traces.StartSpan(ctx)
	defer span.End()

	// IndexedDB data is stored by key "data" pointing to the serialized object. We want to
	// extract that serialized object, and discard the top-level "data" key.
	data, ok := row["data"]
	if !ok {
		return nil, errors.New("row missing top-level data key")
	}

	srcReader := bytes.NewReader(data)

	// First, read the header
	firstTag, _, err := nextPair(srcReader)
	if err != nil {
		return nil, fmt.Errorf("reading header pair: %w", err)
	}
	if firstTag != tagHeader {
		return nil, fmt.Errorf("unknown header tag %x", firstTag)
	}

	// Next up should be our top-level object
	objectTag, _, err := nextPair(srcReader)
	if err != nil {
		return nil, fmt.Errorf("reading top-level object tag: %w", err)
	}
	if objectTag != tagObjectObject {
		return nil, fmt.Errorf("object not found after header: expected %x, got %x", tagObjectObject, objectTag)
	}

	// Read all entries in our object
	resultObj, err := deserializeObject(srcReader)
	if err != nil {
		return nil, fmt.Errorf("reading top-level object: %w", err)
	}

	return resultObj, nil
}

// nextPair returns the next (tag, data) pair from `srcReader`.
func nextPair(srcReader *bytes.Reader) (uint32, uint32, error) {
	// Tags and data are written as a singular little-endian uint64 value.
	// For example, the pair (`tagBoolean`, 1) is written as 01 00 00 00 02 00 FF FF,
	// where 0xffff0002 is `tagBoolean`.
	// To read the pair, we read the next 8 bytes in reverse order, treating the
	// first four as the tag and the next four as the data.
	var err error
	pairBytes := make([]byte, 8)
	for i := 7; i >= 0; i -= 1 {
		pairBytes[i], err = srcReader.ReadByte()
		if err != nil {
			return 0, 0, fmt.Errorf("reading byte in pair: %w", err)
		}
	}

	return binary.BigEndian.Uint32(pairBytes[0:4]), binary.BigEndian.Uint32(pairBytes[4:]), nil
}

// deserializeObject deserializes the next object from `srcReader`.
func deserializeObject(srcReader *bytes.Reader) (map[string][]byte, error) {
	resultObj := make(map[string][]byte, 0)

	for {
		nextObjTag, nextObjData, err := nextPair(srcReader)
		if err != nil {
			return nil, fmt.Errorf("reading next pair in object: %w", err)
		}

		if nextObjTag == tagEndOfKeys {
			// All done! Return object
			break
		}

		// Read key
		if nextObjTag != tagString {
			return nil, fmt.Errorf("unsupported key type %x", nextObjTag)
		}
		nextKey, err := deserializeString(nextObjData, srcReader)
		if err != nil {
			return nil, fmt.Errorf("reading string for tag %x: %w", nextObjTag, err)
		}
		nextKeyStr := string(nextKey)

		// Read value
		valTag, valData, err := nextPair(srcReader)
		if err != nil {
			return nil, fmt.Errorf("reading next pair for value in object: %w", err)
		}
		valDeserialized, err := deserializeNext(valTag, valData, srcReader)
		if err != nil {
			return nil, fmt.Errorf("deserializing value for key `%s`: %w", nextKeyStr, err)
		}
		resultObj[nextKeyStr] = valDeserialized
	}

	return resultObj, nil
}

// deserializeNext deserializes the item with the given tag `itemTag` and its associated data.
// Depending on the type indicated by `itemTag`, it may read additional data from `srcReader`
// to complete deserializing the item.
func deserializeNext(itemTag uint32, itemData uint32, srcReader *bytes.Reader) ([]byte, error) {
	switch itemTag {
	case tagInt32:
		return []byte(strconv.Itoa(int(itemData))), nil
	case tagNumberObject:
		// Number objects are stored as follows:
		// * first, tagNumberObject with valData `0`
		// * next, a double
		// So, we want to ignore our current `valData`, and read the next pair as a double.
		var d float64
		if err := binary.Read(srcReader, binary.NativeEndian, &d); err != nil {
			return nil, fmt.Errorf("decoding double: %w", err)
		}
		return []byte(strconv.FormatFloat(d, 'f', -1, 64)), nil
	case tagBigInt, tagBigIntObject:
		return deserializeBigInt(itemData, srcReader)
	case tagString, tagStringObject:
		return deserializeString(itemData, srcReader)
	case tagBoolean, tagBooleanObject:
		if itemData > 0 {
			return []byte("true"), nil
		} else {
			return []byte("false"), nil
		}
	case tagDateObject:
		// Date objects are stored as follows:
		// * first, a tagDateObject with valData `0`
		// * next, a double
		// So, we want to ignore our current `valData`, and read the next pair as a double.
		var d float64
		if err := binary.Read(srcReader, binary.NativeEndian, &d); err != nil {
			return nil, fmt.Errorf("decoding double: %w", err)
		}
		// d is milliseconds since epoch
		return []byte(time.UnixMilli(int64(d)).UTC().String()), nil
	case tagRegexpObject:
		return deserializeRegexp(itemData, srcReader)
	case tagObjectObject:
		return deserializeNestedObject(srcReader)
	case tagArrayObject:
		return deserializeArray(itemData, srcReader)
	case tagMapObject:
		return deserializeMap(srcReader)
	case tagSetObject:
		return deserializeSet(srcReader)
	case tagNull, tagUndefined:
		return nil, nil
	case tagArrayBufferObj:
		return nil, errors.New("parsing not implemented for array buffer object")
	case tagTypedArrayObj:
		return nil, errors.New("parsing not implemented for typed array object")
	case tagDataViewObj:
		return nil, errors.New("parsing not implemented for data view object")
	case tagErrorObj:
		return nil, errors.New("parsing not implemented for error object")
	case tagResizableArrayBufferObj:
		return nil, errors.New("parsing not implemented for resizable array buffer object")
	case tagGrowableSharedArrayBufferObj:
		return nil, errors.New("parsing not implemented for growable shared array buffer object")
	default:
		if itemTag >= tagFloatMax {
			return nil, fmt.Errorf("unknown tag type `%x` with data `%d`", itemTag, itemData)
		}

		// We want to reinterpret (itemTag, itemData) as a single double value instead.
		// Unread the last 8 bytes so we can re-read them as a double.
		for i := 0; i < 8; i += 1 {
			if err := srcReader.UnreadByte(); err != nil {
				return nil, fmt.Errorf("unreading byte in preparation for reinterpreting tag as double: %w", err)
			}
		}

		var d float64
		if err := binary.Read(srcReader, binary.NativeEndian, &d); err != nil {
			return nil, fmt.Errorf("decoding double: %w", err)
		}
		return []byte(strconv.FormatFloat(d, 'f', -1, 64)), nil
	}
}

func deserializeString(strData uint32, srcReader *bytes.Reader) ([]byte, error) {
	strLen := strData & bitMask(31)
	isAscii := strData & (1 << 31)

	if isAscii != 0 {
		return deserializeAsciiString(strLen, srcReader)
	}

	return deserializeUtf16String(strLen, srcReader)
}

func deserializeAsciiString(strLen uint32, srcReader *bytes.Reader) ([]byte, error) {
	// Read bytes for string
	var i uint32
	var err error
	strBytes := make([]byte, strLen)
	for i = 0; i < strLen; i += 1 {
		strBytes[i], err = srcReader.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("reading byte in string: %w", err)
		}
	}

	// Now, read padding and discard -- data is stored in 8-byte words
	bytesIntoNextWord := strLen % 8
	if bytesIntoNextWord > 0 {
		paddingLen := 8 - bytesIntoNextWord
		for i = 0; i < paddingLen; i += 1 {
			_, _ = srcReader.ReadByte()
		}
	}

	return strBytes, nil
}

func deserializeUtf16String(strLen uint32, srcReader *bytes.Reader) ([]byte, error) {
	// Two bytes per char
	lenToRead := strLen * 2
	var i uint32
	var err error
	strBytes := make([]byte, lenToRead)
	for i = 0; i < lenToRead; i += 1 {
		strBytes[i], err = srcReader.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("reading byte in string: %w", err)
		}
	}

	// Now, read padding and discard -- data is stored in 8-byte words
	bytesIntoNextWord := lenToRead % 8
	if bytesIntoNextWord > 0 {
		paddingLen := 8 - bytesIntoNextWord
		for i = 0; i < paddingLen; i += 1 {
			_, _ = srcReader.ReadByte()
		}
	}

	// Decode string
	utf16Reader := transform.NewReader(bytes.NewReader(strBytes), unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder())
	decoded, err := io.ReadAll(utf16Reader)
	if err != nil {
		return nil, fmt.Errorf("decoding: %w", err)
	}
	return decoded, nil
}

// deserializeBigInt deserializes exactly as much of the upcoming BigInt as necessary
// to get to the next value. We do not actually convert the raw digits to a string,
// since that is proving to be a lot of work -- we just return a placeholder string.
// We can revisit this decision once we determine we actually care about any BigInt values.
func deserializeBigInt(bitfield uint32, srcReader *bytes.Reader) ([]byte, error) {
	// Determine BigInt length from bitfield
	bigIntRawLen := bitfield & bitMask(31)
	if bigIntRawLen == 0 {
		return []byte("0n"), nil
	}

	// Read the raw bytes of the BigInt
	for i := 0; i < int(bigIntRawLen); i++ {
		if _, err := srcReader.ReadByte(); err != nil {
			return nil, fmt.Errorf("reading byte %d of %d for BigInt: %w", i, bigIntRawLen, err)
		}
	}

	// Determine sign for BigInt from bitfield, then return placeholder string
	isNegative := bitfield & (1 << 31)
	if isNegative > 0 {
		return []byte("-?n"), nil
	}
	return []byte("?n"), nil
}

// Please note that these values are NOT identical to the ones used by Chrome -- global
// and ignorecase are swapped. Flag values retrieved from https://searchfox.org/mozilla-central/source/js/public/RegExpFlags.h.
const (
	regexFlagIgnoreCase  = 0b00000001 // /i
	regexFlagGlobal      = 0b00000010 // /g
	regexFlagMultiline   = 0b00000100 // /m
	regexFlagSticky      = 0b00001000 // /y
	regexFlagUnicode     = 0b00010000 // /u
	regexFlagDotAll      = 0b00100000 // /s
	regexFlagHasIndices  = 0b01000000 // /d
	regexFlagUnicodeSets = 0b10000000 // /v
)

// deserializeRegexp deserializes a regular expression, which is stored as follows:
// * first, a tagRegexpObject with corresponding data indicating the regex flags
// * next, a tagString with corresponding data indicating the regex itself
func deserializeRegexp(regexpData uint32, srcReader *bytes.Reader) ([]byte, error) {
	// First, parse the flags
	flags := make([]byte, 0)
	if regexpData&regexFlagIgnoreCase != 0 {
		flags = append(flags, []byte("i")...)
	}
	if regexpData&regexFlagGlobal != 0 {
		flags = append(flags, []byte("g")...)
	}
	if regexpData&regexFlagMultiline != 0 {
		flags = append(flags, []byte("m")...)
	}
	if regexpData&regexFlagSticky != 0 {
		flags = append(flags, []byte("y")...)
	}
	if regexpData&regexFlagUnicode != 0 {
		flags = append(flags, []byte("u")...)
	}
	if regexpData&regexFlagDotAll != 0 {
		flags = append(flags, []byte("s")...)
	}
	if regexpData&regexFlagHasIndices != 0 {
		flags = append(flags, []byte("d")...)
	}
	if regexpData&regexFlagUnicodeSets != 0 {
		flags = append(flags, []byte("v")...)
	}

	// Now, read the next string to get the regex
	nextTag, nextData, err := nextPair(srcReader)
	if err != nil {
		return nil, fmt.Errorf("reading next pair as string for regex object: %w", err)
	}
	if nextTag != tagString {
		return nil, fmt.Errorf("regex tag followed by unexpected tag `%x` (expected `%x`, tagString)", nextTag, tagString)
	}
	regexStrBytes, err := deserializeString(nextData, srcReader)
	if err != nil {
		return nil, fmt.Errorf("deserializing string portion of regex: %w", err)
	}

	regexFull := append([]byte("/"), regexStrBytes...)
	regexFull = append(regexFull, []byte("/")...)
	regexFull = append(regexFull, flags...)

	return regexFull, nil
}

func deserializeArray(arrayLength uint32, srcReader *bytes.Reader) ([]byte, error) {
	resultArr := make([]any, arrayLength)

	for {
		// The next pair is the index.
		idxTag, idx, err := nextPair(srcReader)
		if err != nil {
			return nil, fmt.Errorf("reading next index in array: %w", err)
		}

		if idxTag == tagEndOfKeys {
			break
		}

		// Now, read the data for this index.
		itemTag, itemData, err := nextPair(srcReader)
		if err != nil {
			return nil, fmt.Errorf("reading item at index %d in array: %w", idx, err)
		}
		arrayItem, err := deserializeNext(itemTag, itemData, srcReader)
		if err != nil {
			return nil, fmt.Errorf("reading item at index %d in array: %w", idx, err)
		}
		resultArr[idx] = string(arrayItem) // cast to string so it's readable when marshalled again below
	}

	arrBytes, err := json.Marshal(resultArr)
	if err != nil {
		return nil, fmt.Errorf("marshalling array: %w", err)
	}

	return arrBytes, nil
}

func deserializeNestedObject(srcReader *bytes.Reader) ([]byte, error) {
	nestedObj, err := deserializeObject(srcReader)
	if err != nil {
		return nil, fmt.Errorf("deserializing nested object: %w", err)
	}

	// Make nested object values readable -- cast []byte to string
	readableNestedObj := make(map[string]string)
	for k, v := range nestedObj {
		readableNestedObj[k] = string(v)
	}

	resultObj, err := json.Marshal(readableNestedObj)
	if err != nil {
		return nil, fmt.Errorf("marshalling nested object: %w", err)
	}

	return resultObj, nil
}

// deserializeMap is similar to deserializeNestedObject -- except the keys can be complex objects instead of only strings.
// Data is stored in the following format:
// <map tag, 0>
// <key1 tag, key1 tag data>
// <value1 tag, value1 tag data>
// ...key1 fields...
// <tagEndOfKeys, 0> (signals end of key1)
// ...value1 fields...
// <tagEndOfKeys, 0> (signals end of value1)
// ...continue for other key-val pairs...
// <tagEndOfKeys, 0> (signals end of Map)
func deserializeMap(srcReader *bytes.Reader) ([]byte, error) {
	mapObject := make(map[string]string)

	for {
		keyTag, keyData, err := nextPair(srcReader)
		if err != nil {
			return nil, fmt.Errorf("reading next pair for key in map: %w", err)
		}

		if keyTag == tagEndOfKeys {
			// All done! Return map
			break
		}

		valTag, valData, err := nextPair(srcReader)
		if err != nil {
			return nil, fmt.Errorf("reading next pair for value in map: %w", err)
		}

		// Now process all fields for key obj until we hit tagEndOfKeys
		keyBytes, err := deserializeNext(keyTag, keyData, srcReader)
		if err != nil {
			return nil, fmt.Errorf("deserializing key in map: %w", err)
		}

		// Now process all fields for val obj until we hit tagEndOfKeys
		valBytes, err := deserializeNext(valTag, valData, srcReader)
		if err != nil {
			return nil, fmt.Errorf("deserializing value in map for key `%s`: %w", string(keyBytes), err)
		}

		mapObject[string(keyBytes)] = string(valBytes)

		// All done processing current keyTag, valTag -- iterate!
	}

	resultObj, err := json.Marshal(mapObject)
	if err != nil {
		return nil, fmt.Errorf("marshalling map: %w", err)
	}

	return resultObj, nil
}

// deserializeSet is similar to deserializeMap, just without the keys.
func deserializeSet(srcReader *bytes.Reader) ([]byte, error) {
	setObject := make(map[string]struct{})

	for {
		keyTag, keyData, err := nextPair(srcReader)
		if err != nil {
			return nil, fmt.Errorf("reading next pair for key in set: %w", err)
		}

		if keyTag == tagEndOfKeys {
			// All done! Return map
			break
		}

		// Now process all fields for key obj until we hit tagEndOfKeys
		keyBytes, err := deserializeNext(keyTag, keyData, srcReader)
		if err != nil {
			return nil, fmt.Errorf("deserializing key in map: %w", err)
		}

		setObject[string(keyBytes)] = struct{}{}

		// All done processing current keyTag, valTag -- iterate!
	}

	resultObj, err := json.Marshal(setObject)
	if err != nil {
		return nil, fmt.Errorf("marshalling set: %w", err)
	}

	return resultObj, nil
}

func bitMask(n uint32) uint32 {
	return (1 << n) - 1
}
