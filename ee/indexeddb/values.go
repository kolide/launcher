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
	// strings
	tokenAsciiStr byte = 0x22 // "
	tokenUtf16Str byte = 0x63 // c
	// types: object
	tokenObjectBegin byte = 0x6f // o
	tokenObjectEnd   byte = 0x7b // {
	// types: array
	tokenBeginSparseArray byte = 0x61 // a
	tokenEndSparseArray   byte = 0x40 // @
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

	// First, read through the header to extract top-level data
	version, err := readHeader(srcReader)
	if err != nil {
		return nil, fmt.Errorf("reading header: %w", err)
	}

	// Now, parse the actual data in this row
	objData, err := deserializeObject(ctx, slogger, srcReader)
	if err != nil {
		return nil, fmt.Errorf("decoding obj for indexeddb version %d: %w", version, err)
	}

	return objData, nil
}

// readHeader reads through the header bytes at the start of `srcReader`.
// It parses the version, if found. It stops as soon as it reaches the first
// object reference.
func readHeader(srcReader io.ByteReader) (uint64, error) {
	var version uint64
	for {
		nextByte, err := srcReader.ReadByte()
		if err != nil {
			if err == io.EOF {
				return 0, fmt.Errorf("unexpected EOF reading header")
			}
			return 0, fmt.Errorf("reading next byte: %w", err)
		}

		switch nextByte {
		case tokenVersion:
			version, err = binary.ReadUvarint(srcReader)
			if err != nil {
				return 0, fmt.Errorf("decoding uint32: %w", err)
			}
		case tokenObjectBegin:
			// Done reading header
			return version, nil
		default:
			// Padding -- ignore
			continue
		}
	}
}

// deserializeObject deserializes the next object from the srcReader.
func deserializeObject(ctx context.Context, slogger *slog.Logger, srcReader io.ByteReader) (map[string][]byte, error) {
	obj := make(map[string][]byte)

	for {
		// Parse the next property in this object.

		// First, we'll want the object property name. Typically, we'll get " (denoting a string),
		// then the length of the string, then the string itself.
		objPropertyStart, err := srcReader.ReadByte()
		if err != nil {
			return obj, fmt.Errorf("reading object property: %w", err)
		}
		// No more properties. We've reached the end of the object -- return.
		if objPropertyStart == tokenObjectEnd {
			// The next byte is `properties_written`, which we don't care about -- read it
			// so it doesn't affect future parsing.
			_, _ = srcReader.ReadByte()
			return obj, nil
		}

		// Now read the length of the object property name string
		objPropertyNameLen, err := binary.ReadUvarint(srcReader)
		if err != nil {
			return obj, fmt.Errorf("reading uvarint: %w", err)
		}

		// Now read the next `strLen` bytes as the object property name.
		objPropertyBytes := make([]byte, objPropertyNameLen)
		for i := 0; i < int(objPropertyNameLen); i += 1 {
			nextByte, err := srcReader.ReadByte()
			if err != nil {
				return obj, fmt.Errorf("reading next byte in object property name string: %w", err)
			}

			objPropertyBytes[i] = nextByte
		}

		currentPropertyName := string(objPropertyBytes)

		// Now process the object property's value. The next byte will tell us its type.
		nextByte, err := nextNonPaddingByte(srcReader)
		if err != nil {
			return obj, fmt.Errorf("reading next byte: %w", err)
		}

		// Handle the object property value by its type.
		switch nextByte {
		case tokenObjectBegin:
			// Object nested inside this object
			nestedObj, err := deserializeNestedObject(ctx, slogger, srcReader)
			if err != nil {
				return obj, fmt.Errorf("decoding nested object for %s: %w", currentPropertyName, err)
			}
			obj[currentPropertyName] = nestedObj
		case tokenAsciiStr:
			// ASCII string
			strVal, err := deserializeAsciiStr(srcReader)
			if err != nil {
				return obj, fmt.Errorf("decoding ascii string for %s: %w", currentPropertyName, err)
			}
			obj[currentPropertyName] = strVal
		case tokenUtf16Str:
			// UTF-16 string
			strVal, err := deserializeUtf16Str(srcReader)
			if err != nil {
				return obj, fmt.Errorf("decoding ascii string for %s: %w", currentPropertyName, err)
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
				return obj, fmt.Errorf("decoding int32 for %s: %w", currentPropertyName, err)
			}
			obj[currentPropertyName] = []byte(strconv.Itoa(int(propertyInt)))
		case tokenBeginSparseArray:
			// This is the only type of array I've encountered so far, so it's the only one implemented.
			arr, err := deserializeSparseArray(ctx, slogger, srcReader)
			if err != nil {
				return obj, fmt.Errorf("decoding array for %s: %w", currentPropertyName, err)
			}
			obj[currentPropertyName] = arr
		case tokenPadding, tokenVerifyObjectCount:
			// We don't care about these types
			continue
		default:
			slogger.Log(ctx, slog.LevelWarn,
				"unknown token type",
				"token", fmt.Sprintf("%02x", nextByte),
			)
			continue
		}
	}
}

// nextNonPaddingByte reads from srcReader and discards `tokenPadding` until
// it reaches the next non-padded byte.
func nextNonPaddingByte(srcReader io.ByteReader) (byte, error) {
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

// deserializeSparseArray deserializes the next array from the srcReader.
// Currently, it only handles an array of objects.
func deserializeSparseArray(ctx context.Context, slogger *slog.Logger, srcReader io.ByteReader) ([]byte, error) {
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

func deserializeNestedObject(ctx context.Context, slogger *slog.Logger, srcReader io.ByteReader) ([]byte, error) {
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
func deserializeAsciiStr(srcReader io.ByteReader) ([]byte, error) {
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
func deserializeUtf16Str(srcReader io.ByteReader) ([]byte, error) {
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
