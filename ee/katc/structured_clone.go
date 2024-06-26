package katc

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

const (
	tagFloatMax            uint32 = 0xfff00000
	tagHeader              uint32 = 0xfff10000
	tagNull                uint32 = 0xffff0000
	tagUndefined           uint32 = 0xffff0001
	tagBoolean             uint32 = 0xffff0002
	tagInt32               uint32 = 0xffff0003
	tagString              uint32 = 0xffff0004
	tagDateObject          uint32 = 0xffff0005
	tagRegexpObject        uint32 = 0xffff0006
	tagArrayObject         uint32 = 0xffff0007
	tagObjectObject        uint32 = 0xffff0008
	tagArrayBufferObjectV2 uint32 = 0xffff0009
	tagBooleanObject       uint32 = 0xffff000a
	tagStringObject        uint32 = 0xffff000b
	tagNumberObject        uint32 = 0xffff000c
	tagBackReferenceObject uint32 = 0xffff000d
	tagDoNotUse1           uint32 = 0xffff000e
	tagDoNotUse2           uint32 = 0xffff000f
	tagTypedArrayObjectV2  uint32 = 0xffff0010
	tagMapObject           uint32 = 0xffff0011
	tagSetObject           uint32 = 0xffff0012
	tagEndOfKeys           uint32 = 0xffff0013
)

// structuredCloneDeserialize deserializes a JS object that has been stored in IndexedDB
// by Firefox.
// References:
// * https://stackoverflow.com/a/59923297
// * https://searchfox.org/mozilla-central/source/js/public/StructuredClone.h
// * https://searchfox.org/mozilla-central/source/js/src/vm/StructuredClone.cpp (see especially JSStructuredCloneReader::read)
// * https://html.spec.whatwg.org/multipage/structured-data.html#structureddeserialize
func structuredCloneDeserialize(ctx context.Context, data []byte, slogger *slog.Logger) ([]byte, error) {
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

	// Marshal the object to return
	objRaw, err := json.Marshal(resultObj)
	if err != nil {
		return nil, fmt.Errorf("marshalling result: %w", err)
	}

	return objRaw, nil
}

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

func deserializeObject(srcReader io.ByteReader) (map[string]any, error) {
	resultObj := make(map[string]any, 0)

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

		// Read value
		valTag, valData, err := nextPair(srcReader)
		if err != nil {
			return nil, fmt.Errorf("reading next pair for value in object: %w", err)
		}

		switch valTag {
		case tagInt32:
			resultObj[nextKey] = valData
		case tagString, tagStringObject:
			str, err := deserializeString(valData, srcReader)
			if err != nil {
				return nil, fmt.Errorf("reading string for key %s: %w", nextKey, err)
			}
			resultObj[nextKey] = str
		case tagObjectObject:
			obj, err := deserializeObject(srcReader)
			if err != nil {
				return nil, fmt.Errorf("reading object for key %s: %w", nextKey, err)
			}
			resultObj[nextKey] = obj
		case tagArrayObject:
			arr, err := deserializeArray(valData, srcReader)
			if err != nil {
				return nil, fmt.Errorf("reading array for key %s: %w", nextKey, err)
			}
			resultObj[nextKey] = arr
		case tagNull, tagUndefined:
			resultObj[nextKey] = nil
		default:
			return nil, fmt.Errorf("cannot process %s: unknown tag type %x", nextKey, valTag)
		}
	}

	return resultObj, nil
}

func deserializeString(strData uint32, srcReader io.ByteReader) (string, error) {
	strLen := strData & bitMask(31)
	isAscii := strData & (1 << 31)

	if isAscii != 0 {
		return deserializeAsciiString(strLen, srcReader)
	}

	return deserializeUtf16String(strLen, srcReader)
}

func deserializeAsciiString(strLen uint32, srcReader io.ByteReader) (string, error) {
	// Read bytes for string
	var i uint32
	var err error
	strBytes := make([]byte, strLen)
	for i = 0; i < strLen; i += 1 {
		strBytes[i], err = srcReader.ReadByte()
		if err != nil {
			return "", fmt.Errorf("reading byte in string: %w", err)
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

	return string(strBytes), nil
}

func deserializeUtf16String(strLen uint32, srcReader io.ByteReader) (string, error) {
	// Two bytes per char
	lenToRead := strLen * 2
	var i uint32
	var err error
	strBytes := make([]byte, lenToRead)
	for i = 0; i < lenToRead; i += 1 {
		strBytes[i], err = srcReader.ReadByte()
		if err != nil {
			return "", fmt.Errorf("reading byte in string: %w", err)
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
		return "", fmt.Errorf("decoding: %w", err)
	}
	return string(decoded), nil
}

func deserializeArray(arrayLength uint32, srcReader io.ByteReader) ([]any, error) {
	resultArr := make([]any, arrayLength)

	// We discard the next pair before reading the array.
	_, _, _ = nextPair(srcReader)

	for i := 0; i < int(arrayLength); i += 1 {
		itemTag, _, err := nextPair(srcReader)
		if err != nil {
			return nil, fmt.Errorf("reading item at index %d in array: %w", i, err)
		}

		switch itemTag {
		case tagObjectObject:
			obj, err := deserializeObject(srcReader)
			if err != nil {
				return nil, fmt.Errorf("reading object at index %d in array: %w", i, err)
			}
			resultArr[i] = obj
		default:
			return nil, fmt.Errorf("cannot process item at index %d in array: unsupported tag type %x", i, itemTag)
		}
	}

	return resultArr, nil
}

func bitMask(n uint32) uint32 {
	return (1 << n) - 1
}
