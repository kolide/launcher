package indexeddb

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
	"strings"

	"github.com/kolide/launcher/pkg/traces"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// See: https://github.com/v8/v8/blob/master/src/objects/value-serializer.cc
const (
	// header token
	tokenVersion byte = 0xff
	// booleans
	tokenTrue  byte = 0x54 // T
	tokenFalse byte = 0x46 // F
	// numbers
	tokenInt32  byte = 0x49 // I
	tokenUint32 byte = 0x55 // U
	tokenDouble byte = 0x4e // N
	// strings
	tokenAsciiStr byte = 0x22 // "
	tokenUtf16Str byte = 0x63 // c
	// regex
	tokenRegexp byte = 0x52 // R
	// dates
	tokenDate byte = 0x44 // D
	// types: object
	tokenObjectBegin byte = 0x6f // o
	tokenObjectEnd   byte = 0x7b // {
	// types: map
	tokenMapBegin byte = 0x3b // ;
	tokenMapEnd   byte = 0x3a // :
	// types: set
	tokenSetBegin byte = 0x27 // '
	tokenSetEnd   byte = 0x2c // ,
	// types: array
	tokenBeginSparseArray byte = 0x61 // a
	tokenEndSparseArray   byte = 0x40 // @
	tokenBeginDenseArray  byte = 0x41 // A
	tokenEndDenseArray    byte = 0x24 // $
	// misc
	tokenPadding           byte = 0x00
	tokenVerifyObjectCount byte = 0x3f // ?
	tokenUndefined         byte = 0x5f // _
	tokenNull              byte = 0x30
)

// DeserializeChrome deserializes a JS object that has been stored by Chrome
// in IndexedDB LevelDB-backed databases.
func DeserializeChrome(ctx context.Context, slogger *slog.Logger, row map[string][]byte) (map[string][]byte, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	data, ok := row["data"]
	if !ok {
		return nil, errors.New("row missing top-level data key")
	}
	srcReader := bytes.NewReader(data)

	// First, read the indexeddb version, which precedes the serialized value.
	// See: https://github.com/chromium/chromium/blob/master/content/browser/indexed_db/docs/leveldb_coding_scheme.md#object-store-data
	indexeddbVersion, err := binary.ReadUvarint(srcReader)
	if err != nil {
		return nil, fmt.Errorf("reading uvarint as indexeddb version: %w", err)
	}

	// Next, read through the header to extract top-level data
	serializerVersion, err := readHeader(srcReader)
	if err != nil {
		return nil, fmt.Errorf("reading header with indexeddb version %d and serializer version %d: %w", indexeddbVersion, serializerVersion, err)
	}

	// Now, parse the actual data in this row
	objData, err := deserializeObject(ctx, slogger, srcReader)
	if err != nil {
		return nil, fmt.Errorf("decoding obj for indexeddb version %d and serializer version %d: %w", indexeddbVersion, serializerVersion, err)
	}

	return objData, nil
}

// readHeader reads through the header bytes at the start of `srcReader`,
// after reading the indexeddb version. The header usually contains
// 0xff (tokenVersion), followed by the serializer version (a varint).
// The end of the header is signaled by 0x6f (tokenObjectBegin) -- we stop
// reading the header at this point.
func readHeader(srcReader *bytes.Reader) (uint64, error) {
	var serializerVersion uint64
	for {
		nextByte, err := srcReader.ReadByte()
		if err != nil {
			if err == io.EOF {
				return serializerVersion, errors.New("unexpected EOF reading header")
			}
			return serializerVersion, fmt.Errorf("reading next byte in header: %w", err)
		}

		switch nextByte {
		case tokenVersion:
			// Read the version first. It is fine to overwrite version if we saw the version token
			// before -- the last instance of the version token is the correct one.
			serializerVersion, err = binary.ReadUvarint(srcReader)
			if err != nil {
				return serializerVersion, fmt.Errorf("decoding uint32 for version in header: %w", err)
			}
		case tokenObjectBegin:
			// Done reading header
			return serializerVersion, nil
		default:
			// Padding -- ignore
		}
	}
}

// deserializeObject deserializes the next object from the srcReader.
func deserializeObject(ctx context.Context, slogger *slog.Logger, srcReader *bytes.Reader) (map[string][]byte, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	obj := make(map[string][]byte)

	for {
		// Parse the next property in this object.

		// First, we'll want the object property name. Typically, we'll get " (denoting a string),
		// then the length of the string, then the string itself.
		objPropertyStart, err := nextNonPaddingByte(srcReader)
		if err != nil {
			return obj, fmt.Errorf("reading object property: %w", err)
		}

		var currentPropertyName string
		switch objPropertyStart {
		case tokenObjectEnd:
			// No more properties. We've reached the end of the object -- return.
			// The next byte is `properties_written`, which we don't care about -- read it
			// so it doesn't affect future parsing.
			_, _ = srcReader.ReadByte()
			return obj, nil
		case tokenAsciiStr:
			objectPropertyNameBytes, err := deserializeAsciiStr(srcReader)
			if err != nil {
				return obj, fmt.Errorf("deserializing object property ascii string: %w", err)
			}
			currentPropertyName = string(objectPropertyNameBytes)
		case tokenUtf16Str:
			objectPropertyNameBytes, err := deserializeUtf16Str(srcReader)
			if err != nil {
				return obj, fmt.Errorf("deserializing object property UTF-16 string: %w", err)
			}
			currentPropertyName = string(objectPropertyNameBytes)
		default:
			// Handle unexpected tokens here. Likely, if we run into this issue, we've
			// already committed an error when parsing. Collect as much information as
			// we can, so that we can use logs to troubleshoot the issue.
			i := 0
			objKeys := make([]string, len(obj))
			for k := range obj {
				objKeys[i] = k
				i++
			}
			nextByte, err := srcReader.ReadByte() // peek ahead to see if next byte gives us useful information
			slogger.Log(ctx, slog.LevelWarn,
				"object property name has unexpected non-string type",
				"tag", fmt.Sprintf("%02x", objPropertyStart),
				"current_object_size", len(obj),
				"current_obj_properties", strings.Join(objKeys, ","),
				"unread_byte_count", srcReader.Len(),
				"total_byte_count", srcReader.Size(),
				"next_byte", fmt.Sprintf("%02x", nextByte),
				"next_byte_read_err", err,
			)
			return obj, fmt.Errorf("object property name has unexpected non-string type %02x", objPropertyStart)
		}

		// Now process the object property's value. The next byte will tell us its type.
		nextByte, err := nextNonPaddingByte(srcReader)
		if err != nil {
			return obj, fmt.Errorf("reading next byte for `%s`: %w", currentPropertyName, err)
		}

		// Handle the object property value by its type.
		val, err := deserializeNext(ctx, slogger, nextByte, srcReader)
		if err != nil {
			return obj, fmt.Errorf("decoding value for `%s`: %w", currentPropertyName, err)
		}
		obj[currentPropertyName] = val
	}
}

// nextNonPaddingByte reads from srcReader and discards `tokenPadding` until
// it reaches the next non-padded byte.
func nextNonPaddingByte(srcReader *bytes.Reader) (byte, error) {
	for {
		nextByte, err := srcReader.ReadByte()
		if err != nil {
			if err == io.EOF {
				return 0, fmt.Errorf("did not expect EOF reading next byte: %w", err)
			}
			return 0, fmt.Errorf("reading next byte: %w", err)
		}
		if nextByte == tokenPadding {
			continue
		}
		return nextByte, nil
	}
}

func deserializeNext(ctx context.Context, slogger *slog.Logger, nextToken byte, srcReader *bytes.Reader) ([]byte, error) {
	for {
		switch nextToken {
		case tokenObjectBegin:
			return deserializeNestedObject(ctx, slogger, srcReader)
		case tokenAsciiStr:
			return deserializeAsciiStr(srcReader)
		case tokenUtf16Str:
			return deserializeUtf16Str(srcReader)
		case tokenRegexp:
			return deserializeRegexp(srcReader)
		case tokenTrue:
			return []byte("true"), nil
		case tokenFalse:
			return []byte("false"), nil
		case tokenUndefined, tokenNull:
			return nil, nil
		case tokenInt32:
			propertyInt, err := binary.ReadVarint(srcReader)
			if err != nil {
				return nil, fmt.Errorf("decoding int32: %w", err)
			}
			return []byte(strconv.Itoa(int(propertyInt))), nil
		case tokenDouble:
			var d float64
			if err := binary.Read(srcReader, binary.NativeEndian, &d); err != nil {
				return nil, fmt.Errorf("decoding double: %w", err)
			}
			return []byte(strconv.FormatFloat(d, 'f', -1, 64)), nil
		case tokenDate:
			var d float64
			if err := binary.Read(srcReader, binary.NativeEndian, &d); err != nil {
				return nil, fmt.Errorf("decoding double as date: %w", err)
			}
			return []byte(strconv.FormatFloat(d, 'f', -1, 64)), nil
		case tokenBeginSparseArray:
			return deserializeSparseArray(ctx, slogger, srcReader)
		case tokenBeginDenseArray:
			return deserializeDenseArray(ctx, slogger, srcReader)
		case tokenMapBegin:
			return deserializeMap(ctx, slogger, srcReader)
		case tokenSetBegin:
			return deserializeSet(ctx, slogger, srcReader)
		case tokenPadding, tokenVerifyObjectCount:
			// We don't care about these types -- we want to try reading again
			var err error
			nextToken, err = nextNonPaddingByte(srcReader)
			if err != nil {
				return nil, fmt.Errorf("reading next non-padding byte after padding byte: %w", err)
			}
			continue
		default:
			slogger.Log(ctx, slog.LevelWarn,
				"unknown token type, will attempt to keep reading",
				"token", fmt.Sprintf("%02x", nextToken),
			)
			var err error
			nextToken, err = nextNonPaddingByte(srcReader)
			if err != nil {
				return nil, fmt.Errorf("reading next non-padding byte after unknown token byte: %w", err)
			}
		}
	}
}

// deserializeSparseArray deserializes the next sparse array from the srcReader.
func deserializeSparseArray(ctx context.Context, slogger *slog.Logger, srcReader *bytes.Reader) ([]byte, error) {
	// After an array start, the next byte will be the length of the array.
	arrayLen, err := binary.ReadUvarint(srcReader)
	if err != nil {
		return nil, fmt.Errorf("reading uvarint: %w", err)
	}

	// Read from srcReader until we've filled the array to the correct size.
	arrItems := make([]any, arrayLen)
	reachedEndOfArray := false
	for {
		idxByte, err := srcReader.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("reading next byte: %w", err)
		}

		// First, get the index for this item in the array
		var i int
		switch idxByte {
		case tokenInt32:
			arrIdx, err := binary.ReadVarint(srcReader)
			if err != nil {
				return nil, fmt.Errorf("reading varint: %w", err)
			}
			i = int(arrIdx)
		case tokenUint32:
			arrIdx, err := binary.ReadUvarint(srcReader)
			if err != nil {
				return nil, fmt.Errorf("reading uvarint: %w", err)
			}
			i = int(arrIdx)
		case tokenEndSparseArray:
			// We have extra padding here -- the next two bytes are `properties_written` and `length`,
			// respectively. We don't care about checking them, so we read and discard them.
			_, _ = srcReader.ReadByte()
			_, _ = srcReader.ReadByte()
			// The array has ended -- return.
			reachedEndOfArray = true
		case 0x01, 0x03:
			// This occurs immediately before tokenEndSparseArray -- not sure why. We can ignore it.
			continue
		default:
			return nil, fmt.Errorf("unexpected array index type: 0x%02x / `%s`", idxByte, string(idxByte))
		}

		if reachedEndOfArray {
			break
		}

		// Now read item at index
		nextByte, err := srcReader.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("reading next byte: %w", err)
		}
		switch nextByte {
		case tokenObjectBegin:
			obj, err := deserializeNestedObject(ctx, slogger, srcReader)
			if err != nil {
				return nil, fmt.Errorf("decoding object in array: %w", err)
			}
			arrItems[i] = string(obj) // cast to string so it's readable when marshalled again below
		case tokenAsciiStr:
			str, err := deserializeAsciiStr(srcReader)
			if err != nil {
				return nil, fmt.Errorf("decoding string in array: %w", err)
			}
			arrItems[i] = string(str) // cast to string so it's readable when marshalled again below
		default:
			return nil, fmt.Errorf("unimplemented array item type 0x%02x / `%s`", nextByte, string(nextByte))
		}
	}

	arrBytes, err := json.Marshal(arrItems)
	if err != nil {
		return nil, fmt.Errorf("marshalling array: %w", err)
	}

	return arrBytes, nil
}

// deserializeDenseArray deserializes the next dense array from the srcReader.
// Dense arrays are arrays of items that are NOT paired with indices, as in sparse arrays.
func deserializeDenseArray(ctx context.Context, slogger *slog.Logger, srcReader *bytes.Reader) ([]byte, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	// After an array start, the next byte will be the length of the array.
	arrayLen, err := binary.ReadUvarint(srcReader)
	if err != nil {
		return nil, fmt.Errorf("reading uvarint: %w", err)
	}

	// Read from srcReader until we've filled the array to the correct size.
	arrItems := make([]any, 0)
	reachedEndOfArray := false
	for {
		if reachedEndOfArray {
			break
		}

		// Read item at index
		nextByte, err := srcReader.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("reading next byte: %w", err)
		}
		switch nextByte {
		case tokenObjectBegin:
			obj, err := deserializeNestedObject(ctx, slogger, srcReader)
			if err != nil {
				return nil, fmt.Errorf("decoding object in array of length %d: %w", arrayLen, err)
			}
			arrItems = append(arrItems, string(obj)) // cast to string so it's readable when marshalled again below
		case tokenAsciiStr:
			str, err := deserializeAsciiStr(srcReader)
			if err != nil {
				return nil, fmt.Errorf("decoding string in array of length %d: %w", arrayLen, err)
			}
			arrItems = append(arrItems, string(str)) // cast to string so it's readable when marshalled again below
		case tokenEndDenseArray:
			// We have extra padding here -- the next two bytes are `properties_written` and `length`,
			// respectively. We don't care about checking them, so we read and discard them.
			_, _ = srcReader.ReadByte()
			_, _ = srcReader.ReadByte()
			reachedEndOfArray = true
		case 0x01, 0x03:
			// This occurs immediately before tokenEndSparseArray -- not sure why. We can ignore it.
			continue
		default:
			return nil, fmt.Errorf("unimplemented array item type 0x%02x / `%s` in array of length %d", nextByte, string(nextByte), arrayLen)
		}
	}

	arrBytes, err := json.Marshal(arrItems)
	if err != nil {
		return nil, fmt.Errorf("marshalling array: %w", err)
	}

	return arrBytes, nil
}

func deserializeNestedObject(ctx context.Context, slogger *slog.Logger, srcReader *bytes.Reader) ([]byte, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	nestedObj, err := deserializeObject(ctx, slogger, srcReader)
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

// deserializeMap deserializes a JS map. The map is a bunch of items in a row, where the first item
// is a key and the item after it is its corresponding value, and so on until we read `tokenMapEnd`.
func deserializeMap(ctx context.Context, slogger *slog.Logger, srcReader *bytes.Reader) ([]byte, error) {
	mapObject := make(map[string]string)

	for {
		// Check to see if we're done with the map
		tokenByteForKey, err := nextNonPaddingByte(srcReader)
		if err != nil {
			return nil, fmt.Errorf("reading next byte: %w", err)
		}
		if tokenByteForKey == tokenMapEnd {
			// All done with the map! Read the length and break
			_, _ = srcReader.ReadByte()
			break
		}

		keyObj, err := deserializeNext(ctx, slogger, tokenByteForKey, srcReader)
		if err != nil {
			return nil, fmt.Errorf("deserializing map key: %w", err)
		}

		tokenByteForValue, err := nextNonPaddingByte(srcReader)
		if err != nil {
			return nil, fmt.Errorf("reading next byte: %w", err)
		}

		valObj, err := deserializeNext(ctx, slogger, tokenByteForValue, srcReader)
		if err != nil {
			return nil, fmt.Errorf("deserializing map value: %w", err)
		}

		mapObject[string(keyObj)] = string(valObj)
	}

	resultMap, err := json.Marshal(mapObject)
	if err != nil {
		return nil, fmt.Errorf("marshalling map: %w", err)
	}

	return resultMap, nil
}

// deserializeSet deserializes a JS set. The set is just a bunch of items in a row
// until we reach `tokenSetEnd`.
func deserializeSet(ctx context.Context, slogger *slog.Logger, srcReader *bytes.Reader) ([]byte, error) {
	setObject := make(map[string]struct{})

	for {
		// Check to see if we're done with the map
		nextToken, err := nextNonPaddingByte(srcReader)
		if err != nil {
			return nil, fmt.Errorf("reading next byte: %w", err)
		}
		if nextToken == tokenSetEnd {
			// All done with the set! Read the length and break
			_, _ = srcReader.ReadByte()
			break
		}

		nextSetObj, err := deserializeNext(ctx, slogger, nextToken, srcReader)
		if err != nil {
			return nil, fmt.Errorf("deserializing next item in set: %w", err)
		}

		setObject[string(nextSetObj)] = struct{}{}
	}

	resultMap, err := json.Marshal(setObject)
	if err != nil {
		return nil, fmt.Errorf("marshalling set: %w", err)
	}

	return resultMap, nil
}

// deserializeAsciiStr handles the upcoming ascii string in srcReader.
func deserializeAsciiStr(srcReader *bytes.Reader) ([]byte, error) {
	strLen, err := binary.ReadUvarint(srcReader)
	if err != nil {
		return nil, fmt.Errorf("reading uvarint: %w", err)
	}

	strBytes := make([]byte, strLen)
	for i := 0; i < int(strLen); i += 1 {
		nextByte, err := srcReader.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("reading next byte at index %d in string with length %d: %w", i, strLen, err)
		}

		strBytes[i] = nextByte
	}

	return strBytes, nil
}

// deserializeUtf16Str handles the upcoming utf-16 string in srcReader.
func deserializeUtf16Str(srcReader *bytes.Reader) ([]byte, error) {
	strLen, err := binary.ReadUvarint(srcReader)
	if err != nil {
		return nil, fmt.Errorf("reading uvarint: %w", err)
	}

	strBytes := make([]byte, strLen)
	for i := 0; i < int(strLen); i += 1 {
		nextByte, err := srcReader.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("reading next byte at index %d in string with length %d: %w", i, strLen, err)
		}

		strBytes[i] = nextByte
	}

	utf16Reader := transform.NewReader(bytes.NewReader(strBytes), unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder())
	decoded, err := io.ReadAll(utf16Reader)
	if err != nil {
		return nil, fmt.Errorf("reading string as utf-16: %w", err)
	}

	return decoded, nil
}

// Please note that these values are NOT identical to the ones used by Firefox -- global
// and ignorecase are swapped. Flag values retrieved from https://github.com/v8/v8/blob/main/src/regexp/regexp-flags.h.
const (
	regexFlagGlobal      = 0b00000001 // /g
	regexFlagIgnoreCase  = 0b00000010 // /i
	regexFlagMultiline   = 0b00000100 // /m
	regexFlagSticky      = 0b00001000 // /y
	regexFlagUnicode     = 0b00010000 // /u
	regexFlagDotAll      = 0b00100000 // /s
	regexFlagHasIndices  = 0b01000000 // /d
	regexFlagUnicodeSets = 0b10000000 // /v
)

// deserializeRegexp handles the upcoming regular expression in srcReader.
// The data takes the following form:
// * tokenAsciiStr
// * byteLength:uint32_t
// * raw data (the regex)
// * flags:uint32_t
func deserializeRegexp(srcReader *bytes.Reader) ([]byte, error) {
	// Read in the string portion of the regexp
	nextByte, err := nextNonPaddingByte(srcReader)
	if err != nil {
		return nil, fmt.Errorf("reading first byte of regexp object: %w", err)
	}
	if nextByte != tokenAsciiStr {
		return nil, fmt.Errorf("unexpected tag 0x%02x / `%s` at start of regexp object (expected 0x%02x / `%s`)", nextByte, string(nextByte), tokenAsciiStr, string(tokenAsciiStr))
	}
	regexpStrBytes, err := deserializeAsciiStr(srcReader)
	if err != nil {
		return nil, fmt.Errorf("deserializing string portion of regexp: %w", err)
	}

	// Read in the flags
	regexpFlags, err := binary.ReadUvarint(srcReader)
	if err != nil {
		return nil, fmt.Errorf("reading uvarint as regexp flag: %w", err)
	}
	flags := make([]byte, 0)
	if regexpFlags&regexFlagIgnoreCase != 0 {
		flags = append(flags, []byte("i")...)
	}
	if regexpFlags&regexFlagGlobal != 0 {
		flags = append(flags, []byte("g")...)
	}
	if regexpFlags&regexFlagMultiline != 0 {
		flags = append(flags, []byte("m")...)
	}
	if regexpFlags&regexFlagSticky != 0 {
		flags = append(flags, []byte("y")...)
	}
	if regexpFlags&regexFlagUnicode != 0 {
		flags = append(flags, []byte("u")...)
	}
	if regexpFlags&regexFlagDotAll != 0 {
		flags = append(flags, []byte("s")...)
	}
	if regexpFlags&regexFlagHasIndices != 0 {
		flags = append(flags, []byte("d")...)
	}
	if regexpFlags&regexFlagUnicodeSets != 0 {
		flags = append(flags, []byte("v")...)
	}

	regexFull := append([]byte("/"), regexpStrBytes...)
	regexFull = append(regexFull, []byte("/")...)
	regexFull = append(regexFull, flags...)

	return regexFull, nil
}
