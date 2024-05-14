package indexeddb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

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

func deserializeIndexeddbValue(src []byte) (map[string]any, error) {
	srcReader := bytes.NewReader(src)
	obj := make(map[string]any)

	// First, read through the header to extract top-level data
	version, err := readHeader(srcReader)
	if err != nil {
		return obj, fmt.Errorf("reading header: %w", err)
	}
	obj["version"] = version

	// Now, parse the actual data in this row
	objData, err := deserializeObject(srcReader)
	obj["data"] = objData
	if err != nil {
		return obj, fmt.Errorf("decoding obj: %w", err)
	}

	return obj, nil
}

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
			// done reading header
			return version, nil
		default:
			// padding -- ignore
			continue
		}
	}
}

func deserializeObject(srcReader io.ByteReader) (map[string]any, error) {
	obj := make(map[string]any)

	for {
		// Parse the next property in this object.
		// First, we'll want the object property name. Typically, we'll get " (denoting a string),
		// then the length of the string, then the string itself.
		objPropertyStart, err := srcReader.ReadByte()
		if err != nil {
			return obj, fmt.Errorf("reading object property: %w", err)
		}
		if objPropertyStart == tokenObjectEnd {
			// The next byte is `properties_written`, which we don't care about -- read it
			// so it doesn't affect future parsing.
			_, _ = srcReader.ReadByte()
			// All done parsing this object! Return it
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
		nextByte, err := srcReader.ReadByte()
		if err != nil {
			if err == io.EOF {
				return obj, nil
			}
			return obj, fmt.Errorf("reading next byte: %w", err)
		}

		// Handle the object property value by its type.
		switch nextByte {
		case tokenObjectBegin:
			// Object nested inside this object
			nestedObj, err := deserializeObject(srcReader)
			if err != nil {
				return obj, fmt.Errorf("decoding nested object: %w", err)
			}
			obj[currentPropertyName] = nestedObj
		case tokenAsciiStr:
			// ASCII string
			strVal, err := deserializeAsciiStr(srcReader)
			if err != nil {
				return obj, fmt.Errorf("decoding ascii string: %w", err)
			}
			obj[currentPropertyName] = strVal
		case tokenUtf16Str:
			// UTF-16 string
			strVal, err := deserializeUtf16Str(srcReader)
			if err != nil {
				return obj, fmt.Errorf("decoding ascii string: %w", err)
			}
			obj[currentPropertyName] = strVal
		case tokenTrue:
			obj[currentPropertyName] = true
		case tokenFalse:
			obj[currentPropertyName] = false
		case tokenUndefined, tokenNull:
			obj[currentPropertyName] = nil
		case tokenInt32:
			propertyInt, err := binary.ReadVarint(srcReader)
			if err != nil {
				return obj, fmt.Errorf("decoding int32: %w", err)
			}
			obj[currentPropertyName] = propertyInt
		case tokenBeginSparseArray:
			// This is the only type of array we seem to encounter
			arr, err := deserializeSparseArray(srcReader)
			if err != nil {
				return obj, fmt.Errorf("decoding array: %w", err)
			}
			obj[currentPropertyName] = arr
		case tokenPadding, tokenVerifyObjectCount:
			continue
		default:
			fmt.Printf("unsure how to handle token 0x%02x / `%s`\n", nextByte, string(nextByte))
		}
	}
}

// deserializeSparseArray currently only handles arrays of objects.
func deserializeSparseArray(srcReader io.ByteReader) ([]any, error) {
	// After an array start, the next byte will be the length of the array.
	arrayLen, err := binary.ReadUvarint(srcReader)
	if err != nil {
		return nil, fmt.Errorf("reading uvarint: %w", err)
	}

	// Read from srcReader until we've filled the array to the correct size.
	arrItems := make([]any, arrayLen)
	for {
		idxByte, err := srcReader.ReadByte()
		if err != nil {
			return arrItems, fmt.Errorf("reading next byte: %w", err)
		}

		// First, get the index for this item in the array
		var i int
		switch idxByte {
		case tokenInt32:
			arrIdx, err := binary.ReadVarint(srcReader)
			if err != nil {
				return arrItems, fmt.Errorf("reading varint: %w", err)
			}
			i = int(arrIdx)
		case tokenUint32:
			arrIdx, err := binary.ReadUvarint(srcReader)
			if err != nil {
				return arrItems, fmt.Errorf("reading uvarint: %w", err)
			}
			i = int(arrIdx)
		case tokenEndSparseArray:
			// We have extra padding here -- the next two bytes are `properties_written` and `length`,
			// respectively. We don't care about checking them, so we read and discard them.
			_, _ = srcReader.ReadByte()
			_, _ = srcReader.ReadByte()
			// The array has ended -- return.
			return arrItems, nil
		case 0x01, 0x03:
			// This occurs immediately before tokenEndSparseArray -- not sure why. We can ignore it.
			continue
		default:
			return arrItems, fmt.Errorf("unexpected array index type: 0x%02x / `%s`", idxByte, string(idxByte))
		}

		// Now read item at index
		nextByte, err := srcReader.ReadByte()
		if err != nil {
			return arrItems, fmt.Errorf("reading next byte: %w", err)
		}
		switch nextByte {
		case tokenObjectBegin:
			obj, err := deserializeObject(srcReader)
			if err != nil {
				return arrItems, fmt.Errorf("decoding object in array: %w", err)
			}
			arrItems[i] = obj
		default:
			return arrItems, fmt.Errorf("unhandled array type 0x%02x / `%s`", nextByte, string(nextByte))
		}
	}
}

func deserializeAsciiStr(srcReader io.ByteReader) (string, error) {
	strLen, err := binary.ReadUvarint(srcReader)
	if err != nil {
		return "", fmt.Errorf("reading uvarint: %w", err)
	}

	strBytes := make([]byte, strLen)
	for i := 0; i < int(strLen); i += 1 {
		nextByte, err := srcReader.ReadByte()
		if err != nil {
			return "", fmt.Errorf("reading next byte in string: %w", err)
		}

		strBytes[i] = nextByte
	}

	return string(strBytes), nil
}

func deserializeUtf16Str(srcReader io.ByteReader) (string, error) {
	strLen, err := binary.ReadUvarint(srcReader)
	if err != nil {
		return "", fmt.Errorf("reading uvarint: %w", err)
	}

	strBytes := make([]byte, strLen)
	for i := 0; i < int(strLen); i += 1 {
		nextByte, err := srcReader.ReadByte()
		if err != nil {
			return "", fmt.Errorf("reading next byte in string: %w", err)
		}

		strBytes[i] = nextByte
	}

	utf16Reader := transform.NewReader(bytes.NewReader(strBytes), unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder())
	decoded, err := io.ReadAll(utf16Reader)
	if err != nil {
		return "", fmt.Errorf("reading as utf-16: %w", err)
	}

	return string(decoded), nil
}
