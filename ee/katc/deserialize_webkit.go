package katc

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"

	"github.com/kolide/launcher/v2/ee/observability"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// deserializeWebkit deserializes a JS object that has been stored by WebKit
// in IndexedDB sqlite-backed databases.
// References:
// * https://github.com/WebKit/webkit/blob/main/Source/WebCore/bindings/js/SerializedScriptValue.cpp
func deserializeWebkit(ctx context.Context, slogger *slog.Logger, row map[string][]byte) ([]map[string][]byte, error) {
	_, span := observability.StartSpan(ctx)
	defer span.End()

	// IndexedDB data is stored by key "data" pointing to the serialized object. We want to
	// extract that serialized object, and discard the top-level "data" key.
	data, ok := row["data"]
	if !ok {
		return nil, errors.New("row missing top-level data key")
	}

	// Hex-decode the data first. We don't use the existing hexDecode transform step
	// because we don't want to strip null bytes.
	v := strings.TrimSuffix(strings.TrimPrefix(string(data), "X'"), "'")
	decodedBytes, err := hex.DecodeString(v)
	if err != nil {
		return nil, fmt.Errorf("hex-decoding data: %w", err)
	}

	// Proceed to deserialization
	wd := newWebkitDeserializer(decodedBytes)
	val, err := wd.Deserialize()
	if err != nil {
		return nil, fmt.Errorf("deserializing: %w", err)
	}
	return val, nil
}

const (
	webkitArrayTag        = 1
	webkitObjectTag       = 2
	webkitUndefinedTag    = 3
	webkitNullTag         = 4
	webkitIntTag          = 5
	webkitZeroTag         = 6
	webkitOneTag          = 7
	webkitFalseTag        = 8
	webkitTrueTag         = 9
	webkitDoubleTag       = 10
	webkitDateTag         = 11
	webkitFileTag         = 12
	webkitFileListTag     = 13
	webkitImageDataTag    = 14
	webkitBlobTag         = 15
	webkitStringTag       = 16
	webkitNumberObjectTag = 28
	webkitSetObjectTag    = 29
	webkitMapObjectTag    = 30

	terminatorTag         = 0xFFFFFFFF
	stringPoolTag         = 0xFFFFFFFE
	nonIndexPropertiesTag = 0xFFFFFFFD

	stringDataIs8BitFlag = 0x80000000
)

type webkitDeserializer struct {
	reader     *bytes.Reader
	stringPool [][]byte // maintain reference to seen and deserialized strings
}

func newWebkitDeserializer(src []byte) *webkitDeserializer {
	return &webkitDeserializer{
		reader:     bytes.NewReader(src),
		stringPool: make([][]byte, 0),
	}
}

func (w *webkitDeserializer) Deserialize() ([]map[string][]byte, error) {
	// First up is the version, a uint32. At time of writing, the most recent version is 15.
	version, err := w.deserializeUint32()
	if err != nil {
		return nil, fmt.Errorf("reading version: %w", err)
	}

	// Next up is a value, which could be one of:
	// Array | Object | Map | Set | Terminal
	// At this time, though, we only support Object.
	tag, err := w.reader.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("reading top-level value tag for version %d: %w", version, err)
	}
	if tag != webkitObjectTag {
		return nil, fmt.Errorf("expected top-level object, got %d tag", tag)
	}

	result, err := w.deserializeWebkitObject()
	if err != nil {
		return nil, fmt.Errorf("deserializing webkit object for version %d: %w", version, err)
	}
	return []map[string][]byte{result}, nil
}

// deserializeObject deserializes the upcoming object, which takes the following format:
// ObjectTag (<name:StringData><value:Value>)* TerminatorTag
func (w *webkitDeserializer) deserializeWebkitObject() (map[string][]byte, error) {
	obj := make(map[string][]byte)
	// Read until we get the terminator tag
	for {
		// First is the key name, which is a StringData.
		name, receivedTerminator, err := w.deserializeStringData()
		if err != nil {
			return nil, fmt.Errorf("deserializing object name: %w", err)
		}
		if receivedTerminator {
			break
		}

		val, err := w.deserializeValue()
		if err != nil {
			return nil, fmt.Errorf("deserializing value for key %s: %w", string(name), err)
		}

		obj[string(name)] = val
	}

	return obj, nil
}

// deserializeValue handles the upcoming `Value` in the reader.
// A value is one of: Array | Object | Map | Set | Terminal
func (w *webkitDeserializer) deserializeValue() ([]byte, error) {
	tag, err := w.reader.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("reading value tag: %w", err)
	}

	switch tag {
	case webkitArrayTag:
		return w.deserializeArray()
	case webkitObjectTag:
		return w.deserializeNestedWebkitObject()
	case webkitMapObjectTag:
		return nil, fmt.Errorf("deserializing maps (tag %d) not yet supported", tag)
	case webkitSetObjectTag:
		return nil, fmt.Errorf("deserializing sets (tag %d) not yet supported", tag)
	// Begin Terminal
	case webkitUndefinedTag:
		return []byte("undefined"), nil
	case webkitNullTag:
		return []byte{}, nil
	case webkitIntTag:
		var d int32
		if err := binary.Read(w.reader, binary.LittleEndian, &d); err != nil {
			return nil, fmt.Errorf("decoding int: %w", err)
		}
		return []byte(strconv.Itoa(int(d))), nil
	case webkitZeroTag:
		return []byte("0"), nil
	case webkitOneTag:
		return []byte("1"), nil
	case webkitFalseTag:
		return []byte("false"), nil
	case webkitTrueTag:
		return []byte("true"), nil
	case webkitDateTag, webkitNumberObjectTag, webkitDoubleTag:
		return w.deserializeDouble()
	case webkitFileTag, webkitFileListTag:
		return nil, fmt.Errorf("deserializing files (tag %d) not yet supported", tag)
	case webkitImageDataTag:
		return nil, fmt.Errorf("deserializing images (tag %d) not yet supported", tag)
	case webkitBlobTag:
		return nil, fmt.Errorf("deserializing blobs (tag %d) not yet supported", tag)
	case webkitStringTag:
		str, _, err := w.deserializeStringData()
		if err != nil {
			return nil, fmt.Errorf("deserializing StringData following StringTag: %w", err)
		}
		return str, nil
	default:
		return nil, fmt.Errorf("value tag %d not yet supported", tag)
	}
}

// deserializeObject deserializes the upcoming object, which takes the following format:
// ObjectTag (<name:StringData><value:Value>)* TerminatorTag
func (w *webkitDeserializer) deserializeNestedWebkitObject() ([]byte, error) {
	obj, err := w.deserializeWebkitObject()
	if err != nil {
		return nil, fmt.Errorf("deserializing nested object: %w", err)
	}

	rawObj, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("marshalling object after deserializing: %w", err)
	}
	return rawObj, nil
}

// deserializeArray handles the upcoming array, which takes the following format:
// <length:uint32_t>(<index:uint32_t><value:Value>)* TerminatorTag (NonIndexPropertiesTag (<name:StringData><value:Value>)*) TerminatorTag
func (w *webkitDeserializer) deserializeArray() ([]byte, error) {
	arrayLength, err := w.deserializeUint32()
	if err != nil {
		return nil, fmt.Errorf("reading array length: %w", err)
	}

	resultArr := make([]any, arrayLength)
	for {
		currentIdx, err := w.deserializeUint32()
		if err != nil {
			return nil, fmt.Errorf("deserializing next index in array: %w", err)
		}
		if currentIdx == terminatorTag {
			break
		}

		if currentIdx >= arrayLength {
			return nil, fmt.Errorf("got unexpected index %d for array with length %d", int(currentIdx), arrayLength)
		}

		currentValue, err := w.deserializeValue()
		if err != nil {
			return nil, fmt.Errorf("deserializing next Value in array at index %d: %w", int(currentIdx), err)
		}
		// We want to use currentIdx, rather than i, to handle sparse arrays where not every index
		// necessarily has a value.
		resultArr[currentIdx] = string(currentValue) // cast to string so it's readable when marshalled again below
	}

	// After the array may be array properties. We don't care about these, but will want to read them
	// to advance to the next value we DO care about.
	propertiesStartTag, err := w.deserializeUint32()
	if err != nil {
		return nil, fmt.Errorf("reading properties tag after array: %w", err)
	}
	if propertiesStartTag == nonIndexPropertiesTag {
		if err := w.readAndDiscardArrayProperties(); err != nil {
			return nil, fmt.Errorf("reading array properties after array: %w", err)
		}
	} else if propertiesStartTag != terminatorTag {
		return nil, fmt.Errorf("expected tag after array terminator to be properties start tag or another terminator tag, but got %d", propertiesStartTag)
	}

	arrBytes, err := json.Marshal(resultArr)
	if err != nil {
		return nil, fmt.Errorf("marshalling array: %w", err)
	}

	return arrBytes, nil
}

func (w *webkitDeserializer) readAndDiscardArrayProperties() error {
	// Read until we get the terminator tag
	for {
		// First is the property name, which is a StringData.
		propName, receivedTerminator, err := w.deserializeStringData()
		if err != nil {
			return fmt.Errorf("deserializing property name: %w", err)
		}
		if receivedTerminator {
			break
		}

		if _, err := w.deserializeValue(); err != nil {
			return fmt.Errorf("deserializing value for property name %s: %w", string(propName), err)
		}
	}
	return nil
}

// deserializeStringData handles the upcoming StringData.
// The first uint32 tells us how to handle the StringData.
// If it's terminatorTag, then we return true to stop processing the previous value.
// If it's stringPoolTag, then we read the string from the pool.
// Otherwise, we extract metadata about the upcoming string from that uint32, and proceed to read it.
func (w *webkitDeserializer) deserializeStringData() ([]byte, bool, error) {
	nextTag, err := w.deserializeUint32()
	if err != nil {
		return nil, false, fmt.Errorf("reading start of StringData: %w", err)
	}

	if nextTag == terminatorTag {
		return nil, true, nil
	}

	// Retrieve the string from our string pool -- we've seen it and stored it already.
	if nextTag == stringPoolTag {
		str, err := w.stringFromStringPool()
		if err != nil {
			return nil, false, fmt.Errorf("reading from string pool after string pool tag: %w", err)
		}
		return str, false, nil
	}

	// This uint32 instead indicates string metadata -- extract it.
	is8Bit := nextTag&stringDataIs8BitFlag != 0 // First bit indicates whether this is an 8-bit string
	nextTag &^= stringDataIs8BitFlag            // The remainder indicates the length

	var numBytes uint32
	var decoder *encoding.Decoder
	if is8Bit {
		numBytes = nextTag
		decoder = charmap.ISO8859_1.NewDecoder() // Latin-1
	} else {
		numBytes = nextTag * 2
		decoder = unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewDecoder()
	}

	// Read in the appropriate bytes for our string
	strBytes := make([]byte, numBytes)
	for i := range numBytes {
		nextByte, err := w.reader.ReadByte()
		if err != nil {
			return nil, false, fmt.Errorf("reading byte %d of %d of string: %w", i+1, numBytes, err)
		}
		strBytes[i] = nextByte
	}

	decodeReader := transform.NewReader(bytes.NewReader(strBytes), decoder)
	decoded, err := io.ReadAll(decodeReader)
	if err != nil {
		return nil, false, fmt.Errorf("decoding: %w", err)
	}

	// Now that we've seen this string for the first time, add it to our string pool
	w.stringPool = append(w.stringPool, decoded)

	return decoded, false, nil
}

// stringFromStringPool retrieves a string from the string pool, given the upcoming
// string pool index. When serializing, the serializer maintains an indexed pool
// of strings that it has seen. If it encounters a string to serialize that it has seen before,
// instead of re-serializing that value, it will write stringPoolTag and then the pool index.
// For example:
//
//	{
//	   "id": "abc",
//	   "uuid": "abc"
//	}
//
// The serializer will serialize "id" and store it in the pool at index 0, then "abc" and store it
// in the pool at index 1, then "uuid" and store it in the pool at index 2. When it reaches the second
// "abc", since it's been seen before, it will instead write out stringPoolTag and then 1.
func (w *webkitDeserializer) stringFromStringPool() ([]byte, error) {
	// First, read in the index. The pool index is stored in 1, 2, or 4 bytes,
	// depending on the current size of the string pool.
	var idx int
	if len(w.stringPool) <= 255 {
		var i uint8
		if err := binary.Read(w.reader, binary.LittleEndian, &i); err != nil {
			return nil, fmt.Errorf("reading uint8 index tag: %w", err)
		}
		idx = int(i)
	} else if len(w.stringPool) <= 65535 {
		var i uint16
		if err := binary.Read(w.reader, binary.LittleEndian, &i); err != nil {
			return nil, fmt.Errorf("reading uint16 index tag: %w", err)
		}
		idx = int(i)
	} else {
		var i uint32
		if err := binary.Read(w.reader, binary.LittleEndian, &i); err != nil {
			return nil, fmt.Errorf("reading uint32 index tag: %w", err)
		}
		idx = int(i)
	}

	if idx >= len(w.stringPool) {
		return nil, fmt.Errorf("requested string at index %d but only %d items in string pool", idx, len(w.stringPool))
	}

	// Retrieve the string from the pool
	return w.stringPool[idx], nil
}

func (w *webkitDeserializer) deserializeUint32() (uint32, error) {
	var d uint32
	if err := binary.Read(w.reader, binary.LittleEndian, &d); err != nil {
		return 0, fmt.Errorf("decoding uint32: %w", err)
	}
	return d, nil
}

func (w *webkitDeserializer) deserializeDouble() ([]byte, error) {
	var d float64
	if err := binary.Read(w.reader, binary.LittleEndian, &d); err != nil {
		return nil, fmt.Errorf("decoding double: %w", err)
	}
	return []byte(strconv.FormatFloat(d, 'f', -1, 64)), nil
}
