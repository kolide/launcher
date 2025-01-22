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

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

const (
	tagHeader        uint32 = 0xfff10000
	tagNull          uint32 = 0xffff0000
	tagUndefined     uint32 = 0xffff0001
	tagBoolean       uint32 = 0xffff0002
	tagInt32         uint32 = 0xffff0003
	tagString        uint32 = 0xffff0004
	tagDateObject    uint32 = 0xffff0005
	tagArrayObject   uint32 = 0xffff0007
	tagObjectObject  uint32 = 0xffff0008
	tagBooleanObject uint32 = 0xffff000a
	tagStringObject  uint32 = 0xffff000b
	tagEndOfKeys     uint32 = 0xffff0013
	tagFloatMax      uint32 = 0xfff00000
)

// deserializeFirefox deserializes a JS object that has been stored by Firefox
// in IndexedDB sqlite-backed databases.
// References:
// * https://stackoverflow.com/a/59923297
// * https://searchfox.org/mozilla-central/source/js/src/vm/StructuredClone.cpp (see especially JSStructuredCloneReader::read)
func deserializeFirefox(ctx context.Context, slogger *slog.Logger, row map[string][]byte) (map[string][]byte, error) {
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
func nextPair(srcReader io.ByteReader) (uint32, uint32, error) {
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
func deserializeObject(srcReader io.ByteReader) (map[string][]byte, error) {
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

		switch valTag {
		case tagInt32:
			resultObj[nextKeyStr] = []byte(strconv.Itoa(int(valData)))
		case tagString, tagStringObject:
			str, err := deserializeString(valData, srcReader)
			if err != nil {
				return nil, fmt.Errorf("reading string for key `%s`: %w", nextKeyStr, err)
			}
			resultObj[nextKeyStr] = str
		case tagBoolean:
			if valData > 0 {
				resultObj[nextKeyStr] = []byte("true")
			} else {
				resultObj[nextKeyStr] = []byte("false")
			}
		case tagDateObject:
			// Date objects are stored as follows:
			// * first, a tagDateObject with valData `0`
			// * next, a double
			// So, we want to ignore our current `valData`, and read the next pair as a double.
			nextTag, nextData, err := nextPair(srcReader)
			if err != nil {
				return nil, fmt.Errorf("reading next pair as date object for key `%s`: %w", nextKeyStr, err)
			}
			d := uint64(nextData) | uint64(nextTag)<<32
			resultObj[nextKeyStr] = []byte(strconv.FormatUint(d, 10))
		case tagObjectObject:
			obj, err := deserializeNestedObject(srcReader)
			if err != nil {
				return nil, fmt.Errorf("reading object for key `%s`: %w", nextKeyStr, err)
			}
			resultObj[nextKeyStr] = obj
		case tagArrayObject:
			arr, err := deserializeArray(valData, srcReader)
			if err != nil {
				return nil, fmt.Errorf("reading array for key `%s`: %w", nextKeyStr, err)
			}
			resultObj[nextKeyStr] = arr
		case tagNull, tagUndefined:
			resultObj[nextKeyStr] = nil
		default:
			if valTag < tagFloatMax {
				// We want to reinterpret (valTag, valData) as a single double value instead
				d := uint64(valData) | uint64(valTag)<<32
				resultObj[nextKeyStr] = []byte(strconv.FormatUint(d, 10))
			} else {
				return nil, fmt.Errorf("cannot process object key `%s`: unknown tag type `%x` with data `%d`", nextKeyStr, valTag, valData)
			}
		}
	}

	return resultObj, nil
}

func deserializeString(strData uint32, srcReader io.ByteReader) ([]byte, error) {
	strLen := strData & bitMask(31)
	isAscii := strData & (1 << 31)

	if isAscii != 0 {
		return deserializeAsciiString(strLen, srcReader)
	}

	return deserializeUtf16String(strLen, srcReader)
}

func deserializeAsciiString(strLen uint32, srcReader io.ByteReader) ([]byte, error) {
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

func deserializeUtf16String(strLen uint32, srcReader io.ByteReader) ([]byte, error) {
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

func deserializeArray(arrayLength uint32, srcReader io.ByteReader) ([]byte, error) {
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

		switch itemTag {
		case tagObjectObject:
			obj, err := deserializeNestedObject(srcReader)
			if err != nil {
				return nil, fmt.Errorf("reading object at index %d in array: %w", idx, err)
			}
			resultArr[idx] = string(obj) // cast to string so it's readable when marshalled again below
		case tagString:
			str, err := deserializeString(itemData, srcReader)
			if err != nil {
				return nil, fmt.Errorf("reading string at index %d in array: %w", idx, err)
			}
			resultArr[idx] = string(str) // cast to string so it's readable when marshalled again below
		default:
			return nil, fmt.Errorf("cannot process item at index %d in array: unsupported tag type %x", idx, itemTag)
		}
	}

	arrBytes, err := json.Marshal(resultArr)
	if err != nil {
		return nil, fmt.Errorf("marshalling array: %w", err)
	}

	return arrBytes, nil
}

func deserializeNestedObject(srcReader io.ByteReader) ([]byte, error) {
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

func bitMask(n uint32) uint32 {
	return (1 << n) - 1
}
