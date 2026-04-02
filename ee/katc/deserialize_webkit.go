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

	terminatorTag = 0xFFFFFFFF
	stringPoolTag = 0xFFFFFFFE

	stringDataIs8BitFlag = 0x80000000
)

type webkitDeserializer struct {
	reader     *bytes.Reader
	stringPool []string // maintain reference to seen/deserialized strings
}

func newWebkitDeserializer(src []byte) *webkitDeserializer {
	return &webkitDeserializer{
		reader:     bytes.NewReader(src),
		stringPool: make([]string, 0),
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
		return nil, fmt.Errorf("deserializing arrays (tag %d) not yet supported", tag)
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
		idx, err := w.readPoolIndex()
		if err != nil {
			return nil, false, fmt.Errorf("reading string pool index: %w", err)
		}
		if idx >= len(w.stringPool) {
			return nil, false, fmt.Errorf("requested string at index %d but only %d items in string pool", idx, len(w.stringPool))
		}
		return []byte(w.stringPool[idx]), false, nil
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

	w.stringPool = append(w.stringPool, string(decoded))

	return decoded, false, nil
}

// readPoolIndex reads the pool index, which is stored in 1, 2, or 4 bytes,
// depending on the current size of the string pool.
func (w *webkitDeserializer) readPoolIndex() (int, error) {
	if len(w.stringPool) <= 255 {
		var i uint8
		if err := binary.Read(w.reader, binary.LittleEndian, &i); err != nil {
			return 0, fmt.Errorf("reading uint8 index tag: %w", err)
		}
		return int(i), nil
	} else if len(w.stringPool) <= 65535 {
		var i uint16
		if err := binary.Read(w.reader, binary.LittleEndian, &i); err != nil {
			return 0, fmt.Errorf("reading uint16 index tag: %w", err)
		}
		return int(i), nil
	} else {
		var i uint32
		if err := binary.Read(w.reader, binary.LittleEndian, &i); err != nil {
			return 0, fmt.Errorf("reading uint32 index tag: %w", err)
		}
		return int(i), nil
	}
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
