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
	// types: object
	tokenObjectBegin byte = 0x6f // o
	tokenObjectEnd   byte = 0x7b // {
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
		switch nextByte {
		case tokenObjectBegin:
			// Object nested inside this object
			nestedObj, err := deserializeNestedObject(ctx, slogger, srcReader)
			if err != nil {
				return obj, fmt.Errorf("decoding nested object for `%s`: %w", currentPropertyName, err)
			}
			obj[currentPropertyName] = nestedObj
		case tokenAsciiStr:
			// ASCII string
			strVal, err := deserializeAsciiStr(srcReader)
			if err != nil {
				return obj, fmt.Errorf("decoding ascii string for `%s`: %w", currentPropertyName, err)
			}
			obj[currentPropertyName] = strVal
		case tokenUtf16Str:
			// UTF-16 string
			strVal, err := deserializeUtf16Str(srcReader)
			if err != nil {
				return obj, fmt.Errorf("decoding ascii string for `%s`: %w", currentPropertyName, err)
			}
			obj[currentPropertyName] = strVal
		case tokenTrue:
			obj[currentPropertyName] = []byte("true")
		case tokenFalse:
			obj[currentPropertyName] = []byte("false")
		case tokenUndefined, tokenNull:
			obj[currentPropertyName] = nil
		case tokenInt32:
			propertyInt, err := binary.ReadVarint(srcReader)
			if err != nil {
				return obj, fmt.Errorf("decoding int32 for `%s`: %w", currentPropertyName, err)
			}
			obj[currentPropertyName] = []byte(strconv.Itoa(int(propertyInt)))
		case tokenDouble:
			var d float64
			if err := binary.Read(srcReader, binary.NativeEndian, &d); err != nil {
				return obj, fmt.Errorf("decoding double for `%s`: %w", currentPropertyName, err)
			}
			obj[currentPropertyName] = []byte(strconv.FormatFloat(d, 'f', -1, 64))
		case tokenBeginSparseArray:
			arr, err := deserializeSparseArray(ctx, slogger, srcReader)
			if err != nil {
				return obj, fmt.Errorf("decoding sparse array for `%s`: %w", currentPropertyName, err)
			}
			obj[currentPropertyName] = arr
		case tokenBeginDenseArray:
			arr, err := deserializeDenseArray(ctx, slogger, srcReader)
			if err != nil {
				return obj, fmt.Errorf("decoding dense array for `%s`: %w", currentPropertyName, err)
			}
			obj[currentPropertyName] = arr
		case tokenPadding, tokenVerifyObjectCount:
			// We don't care about these types
			continue
		default:
			slogger.Log(ctx, slog.LevelWarn,
				"unknown token type",
				"current_property_name", currentPropertyName,
				"token", fmt.Sprintf("%02x", nextByte),
			)
			continue
		}
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
