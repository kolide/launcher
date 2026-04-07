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
	// SerializationTag
	webkitArrayTag                = 1
	webkitObjectTag               = 2
	webkitUndefinedTag            = 3
	webkitNullTag                 = 4
	webkitIntTag                  = 5
	webkitZeroTag                 = 6
	webkitOneTag                  = 7
	webkitFalseTag                = 8
	webkitTrueTag                 = 9
	webkitDoubleTag               = 10
	webkitDateTag                 = 11
	webkitFileTag                 = 12
	webkitFileListTag             = 13
	webkitImageDataTag            = 14
	webkitBlobTag                 = 15
	webkitStringTag               = 16
	webkitRegExpTag               = 18
	webkitObjectReferenceTag      = 19
	webkitArrayBufferTag          = 21
	webkitArrayBufferViewTag      = 22
	webkitArrayBufferTransferTag  = 23
	webkitTrueObjectTag           = 24
	webkitFalseObjectTag          = 25
	webkitStringObjectTag         = 26
	webkitEmptyStringObjectTag    = 27
	webkitNumberObjectTag         = 28
	webkitSetObjectTag            = 29
	webkitMapObjectTag            = 30
	webkitNonMapPropertiesTag     = 31
	webkitNonSetPropertiesTag     = 32
	webkitSharedArrayBufferTag    = 34
	webkitResizableArrayBufferTag = 54
	webkitErrorInstanceTag        = 55

	// ArrayBufferViewSubtag
	// We don't interpret the data in array buffer views currently, but when we do,
	// we'll want these.
	/*
		arrayBufferDataViewSubtag          = 0
		arrayBufferInt8ArraySubtag         = 1
		arrayBufferUint8ArraySubtag        = 2
		arrayBufferUint8ClampedArraySubtag = 3
		arrayBufferInt16ArraySubtag        = 4
		arrayBufferUint16ArraySubtag       = 5
		arrayBufferInt32ArraySubtag        = 6
		arrayBufferUint32ArraySubtag       = 7
		arrayBufferFloat32ArraySubtag      = 8
		arrayBufferFloat64ArraySubtag      = 9
		arrayBufferBigInt64ArraySubtag     = 10
		arrayBufferBigUint64ArraySubtag    = 11
		arrayBufferFloat16ArraySubtag      = 12
	*/

	// SerializableErrorType
	errorTypeError          = 0
	errorTypeEvalError      = 1
	errorTypeRangeError     = 2
	errorTypeReferenceError = 3
	errorTypeSyntaxError    = 4
	errorTypeTypeError      = 5
	errorTypeURIError       = 6

	terminatorTag         = 0xFFFFFFFF
	stringPoolTag         = 0xFFFFFFFE
	nonIndexPropertiesTag = 0xFFFFFFFD

	stringDataIs8BitFlag = 0x80000000
)

type webkitDeserializer struct {
	reader     *bytes.Reader
	stringPool [][]byte // maintain reference to seen and deserialized strings
	objectPool [][]byte // maintain reference to seen and deserialized objects -- see webkit's canBeAddedToObjectPool
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

	result, err := w.deserializeObject()
	if err != nil {
		return nil, fmt.Errorf("deserializing webkit object for version %d: %w", version, err)
	}
	return []map[string][]byte{result}, nil
}

// deserializeObject deserializes the upcoming object, which takes the following format:
// ObjectTag (<name:StringData><value:Value>)* TerminatorTag
func (w *webkitDeserializer) deserializeObject() (map[string][]byte, error) {
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
		return w.deserializeNestedObject()
	case webkitMapObjectTag:
		return w.deserializeMapData()
	case webkitSetObjectTag:
		return w.deserializeSetData()
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
	case webkitDateTag, webkitDoubleTag:
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
			return nil, fmt.Errorf("deserializing StringData following tag %d: %w", tag, err)
		}
		return str, nil
	case webkitRegExpTag:
		return w.deserializeRegexp()
	case webkitObjectReferenceTag:
		return fromPool(w.reader, w.objectPool)
	case webkitArrayBufferViewTag:
		return w.deserializeArrayBufferView()
	case webkitTrueObjectTag:
		val := []byte("true")
		w.objectPool = append(w.objectPool, val)
		return val, nil
	case webkitFalseObjectTag:
		val := []byte("false")
		w.objectPool = append(w.objectPool, val)
		return val, nil
	case webkitStringObjectTag:
		str, _, err := w.deserializeStringData()
		if err != nil {
			return nil, fmt.Errorf("deserializing StringData following tag %d: %w", tag, err)
		}
		// The call to deserializeStringData added this string to the string pool --
		// StringObjects should _also_ live in the object pool, so we add it here.
		w.objectPool = append(w.objectPool, str)
		return str, nil
	case webkitEmptyStringObjectTag:
		emptyStr := []byte("")
		w.objectPool = append(w.objectPool, emptyStr)
		return emptyStr, nil
	case webkitNumberObjectTag:
		dbl, err := w.deserializeDouble()
		if err != nil {
			return nil, fmt.Errorf("deserializing number object: %w", err)
		}
		w.objectPool = append(w.objectPool, dbl)
		return dbl, nil
	case webkitErrorInstanceTag:
		return w.deserializeError()
	default:
		return nil, fmt.Errorf("value tag %d not yet supported", tag)
	}
}

// deserializeObject deserializes the upcoming object, which takes the following format:
// ObjectTag (<name:StringData><value:Value>)* TerminatorTag
func (w *webkitDeserializer) deserializeNestedObject() ([]byte, error) {
	nestedObj, err := w.deserializeObject()
	if err != nil {
		return nil, fmt.Errorf("deserializing nested object: %w", err)
	}

	// Make nested object values readable -- cast []byte to string
	readableNestedObj := make(map[string]string)
	for k, v := range nestedObj {
		readableNestedObj[k] = string(v)
	}

	rawObj, err := json.Marshal(readableNestedObj)
	if err != nil {
		return nil, fmt.Errorf("marshalling object after deserializing: %w", err)
	}

	w.objectPool = append(w.objectPool, rawObj)

	return rawObj, nil
}

// deserializeArray handles the upcoming array, which takes the following format:
// <length:uint32_t>(<index:uint32_t><value:Value>)* TerminatorTag (NonIndexPropertiesTag (<name:StringData><value:Value>)*) TerminatorTag
func (w *webkitDeserializer) deserializeArray() ([]byte, error) {
	arrayLength, err := w.deserializeUint32()
	if err != nil {
		return nil, fmt.Errorf("reading array length: %w", err)
	}

	resultArr := make([]string, arrayLength)
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
		if err := w.readAndDiscardProperties(); err != nil {
			return nil, fmt.Errorf("reading array properties after array: %w", err)
		}
	} else if propertiesStartTag != terminatorTag {
		return nil, fmt.Errorf("expected tag after array terminator to be properties start tag or another terminator tag, but got %d", propertiesStartTag)
	}

	arrBytes, err := json.Marshal(resultArr)
	if err != nil {
		return nil, fmt.Errorf("marshalling array: %w", err)
	}

	w.objectPool = append(w.objectPool, arrBytes)

	return arrBytes, nil
}

func (w *webkitDeserializer) readAndDiscardProperties() error {
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

// deserializeArrayBufferView handles the upcoming array buffer view, which has the following format:
// ArrayBufferViewSubtag <byteOffset:uint64_t> <byteLength:uint64_t> (ArrayBuffer | ObjectReference)
func (w *webkitDeserializer) deserializeArrayBufferView() ([]byte, error) {
	subtag, err := w.reader.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("reading array buffer view subtag: %w", err)
	}

	var byteOffset uint64
	if err := binary.Read(w.reader, binary.LittleEndian, &byteOffset); err != nil {
		return nil, fmt.Errorf("decoding byte offset: %w", err)
	}

	var byteLength uint64
	if err := binary.Read(w.reader, binary.LittleEndian, &byteLength); err != nil {
		return nil, fmt.Errorf("decoding byte length: %w", err)
	}

	// Next is either an array buffer or an object reference -- read the tag to find out.
	nextTag, err := w.reader.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("reading next tag inside array buffer view: %w", err)
	}

	// Get the raw data for the array buffer
	var rawData []byte
	switch nextTag {
	case webkitArrayBufferTag:
		arrBuf, err := w.deserializeArrayBuffer()
		if err != nil {
			return nil, fmt.Errorf("reading array buffer inside array buffer view with subtag %d: %w", subtag, err)
		}
		rawData = arrBuf
	case webkitResizableArrayBufferTag:
		arrBuf, err := w.deserializeResizableArrayBuffer()
		if err != nil {
			return nil, fmt.Errorf("reading resizable array buffer inside array buffer view with subtag %d: %w", subtag, err)
		}
		rawData = arrBuf
	case webkitObjectReferenceTag:
		obj, err := fromPool(w.reader, w.objectPool)
		if err != nil {
			return nil, fmt.Errorf("retrieving object reference from object pool: %w", err)
		}
		rawData = obj
	case webkitArrayBufferTransferTag:
		// ArrayBufferTransferTag <value:uint32_t>
		return nil, fmt.Errorf("ArrayBufferTransfer not yet supported in array buffer view with subtag %d", subtag)
	case webkitSharedArrayBufferTag:
		// SharedArrayBufferTag <value:uint32_t>
		return nil, fmt.Errorf("SharedArrayBuffer not yet supported in array buffer view with subtag %d", subtag)
	default:
		return nil, fmt.Errorf("unsupported tag %d in array buffer view with subtag %d", nextTag, subtag)
	}

	// Next, we _would_ interpret the raw data based on the subtag. For now,
	// though, we're just returning the raw uninterpreted data.
	// The ArrayBufferView must additionally get added to the object pool, even if
	// we've already added the enclosed ArrayBuffer, ObjectReference, etc as well --
	// add it now.
	w.objectPool = append(w.objectPool, rawData)
	return rawData, nil
}

// deserializeArrayBuffer handles the upcoming array buffer, which takes the following format:
// <byteLength:uint64_t> <contents:byte{length}>
func (w *webkitDeserializer) deserializeArrayBuffer() ([]byte, error) {
	var length uint64
	if err := binary.Read(w.reader, binary.LittleEndian, &length); err != nil {
		return nil, fmt.Errorf("decoding content length: %w", err)
	}
	rawData := make([]byte, length)
	if _, err := w.reader.Read(rawData); err != nil {
		return nil, fmt.Errorf("reading %d bytes in array buffer: %w", length, err)
	}
	w.objectPool = append(w.objectPool, rawData)

	return rawData, nil
}

// deserializeResizableArrayBuffer handles the upcoming resizable array buffer, which takes the following format:
// <byteLength:uint64_t> <maxLength:uint64_t> <contents:byte{length}>
func (w *webkitDeserializer) deserializeResizableArrayBuffer() ([]byte, error) {
	var length uint64
	if err := binary.Read(w.reader, binary.LittleEndian, &length); err != nil {
		return nil, fmt.Errorf("decoding content length: %w", err)
	}

	var maxLength uint64
	if err := binary.Read(w.reader, binary.LittleEndian, &maxLength); err != nil {
		return nil, fmt.Errorf("decoding max content length: %w", err)
	}

	rawData := make([]byte, length)
	if _, err := w.reader.Read(rawData); err != nil {
		return nil, fmt.Errorf("reading %d bytes in resizable array buffer: %w", length, err)
	}
	w.objectPool = append(w.objectPool, rawData)

	return rawData, nil
}

// deserializeMapData deserializes the upcoming MapData.
// MapData takes the following format:
// (<key:Value><value:Value>)* NonMapPropertiesTag (<name:StringData><value:Value>)* TerminatorTag
func (w *webkitDeserializer) deserializeMapData() ([]byte, error) {
	mapObject := make(map[string]string)

	// Read until we hit webkitNonMapPropertiesTag
	for {
		// First, peek ahead
		nextTag, err := w.reader.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("reading next tag in map: %w", err)
		}
		if nextTag == webkitNonMapPropertiesTag {
			break
		}

		// Not done reading map yet -- unread the byte
		if err := w.reader.UnreadByte(); err != nil {
			return nil, fmt.Errorf("unreading byte after peeking ahead in map: %w", err)
		}

		// Read the key
		keyRaw, err := w.deserializeValue()
		if err != nil {
			return nil, fmt.Errorf("reading next key in map: %w", err)
		}
		key := string(keyRaw)

		// Read the value
		valueRaw, err := w.deserializeValue()
		if err != nil {
			return nil, fmt.Errorf("reading next value in map for key %s: %w", key, err)
		}
		mapObject[key] = string(valueRaw)
	}

	if err := w.readAndDiscardProperties(); err != nil {
		return nil, fmt.Errorf("reading map properties after map: %w", err)
	}

	resultObj, err := json.Marshal(mapObject)
	if err != nil {
		return nil, fmt.Errorf("marshalling map: %w", err)
	}

	w.objectPool = append(w.objectPool, resultObj)

	return resultObj, nil
}

// deserializeSetData deserializes the upcoming SetData.
// SetData takes the following format:
// (<key:Value>)* NonSetPropertiesTag (<name:StringData><value:Value>)* TerminatorTag
func (w *webkitDeserializer) deserializeSetData() ([]byte, error) {
	setObject := make(map[string]struct{})

	// Read until we hit webkitNonSetPropertiesTag
	for {
		// First, peek ahead
		nextTag, err := w.reader.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("reading next tag in set: %w", err)
		}
		if nextTag == webkitNonSetPropertiesTag {
			break
		}

		// Not done reading set yet -- unread the byte
		if err := w.reader.UnreadByte(); err != nil {
			return nil, fmt.Errorf("unreading byte after peeking ahead in set: %w", err)
		}

		// Read the key
		keyRaw, err := w.deserializeValue()
		if err != nil {
			return nil, fmt.Errorf("reading next key in set: %w", err)
		}

		setObject[string(keyRaw)] = struct{}{}
	}

	if err := w.readAndDiscardProperties(); err != nil {
		return nil, fmt.Errorf("reading set properties after set: %w", err)
	}

	resultObj, err := json.Marshal(setObject)
	if err != nil {
		return nil, fmt.Errorf("marshalling set: %w", err)
	}

	w.objectPool = append(w.objectPool, resultObj)

	return resultObj, nil
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
		str, err := fromPool(w.reader, w.stringPool)
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

// fromPool retrieves an item from the corresponding pool, given the upcoming
// pool index. When serializing, the serializer maintains several indexed pools
// of items that it has seen. If it encounters an item to serialize that it has seen before,
// instead of re-serializing that value, it will write the appropriate pool tag and then the pool index.
// For example:
//
//	{
//	   "id": "abc",
//	   "uuid": "abc"
//	}
//
// The serializer will serialize "id" and store it in the string pool at index 0, then "abc" and store it
// in the pool at index 1, then "uuid" and store it in the pool at index 2. When it reaches the second
// "abc", since it's been seen before, it will instead write out stringPoolTag and then 1.
func fromPool(reader *bytes.Reader, pool [][]byte) ([]byte, error) {
	// First, read in the index. The pool index is stored in 1, 2, or 4 bytes,
	// depending on the current size of the string pool.
	var idx int
	if len(pool) <= 255 {
		var i uint8
		if err := binary.Read(reader, binary.LittleEndian, &i); err != nil {
			return nil, fmt.Errorf("reading uint8 index tag: %w", err)
		}
		idx = int(i)
	} else if len(pool) <= 65535 {
		var i uint16
		if err := binary.Read(reader, binary.LittleEndian, &i); err != nil {
			return nil, fmt.Errorf("reading uint16 index tag: %w", err)
		}
		idx = int(i)
	} else {
		var i uint32
		if err := binary.Read(reader, binary.LittleEndian, &i); err != nil {
			return nil, fmt.Errorf("reading uint32 index tag: %w", err)
		}
		idx = int(i)
	}

	if idx >= len(pool) {
		return nil, fmt.Errorf("requested item at index %d but only %d items in pool", idx, len(pool))
	}

	// Retrieve the item from the pool
	return pool[idx], nil
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

// deserializeRegexp deserializes the upcoming regexp, which takes the following format:
// <pattern:StringData><flags:StringData>
func (w *webkitDeserializer) deserializeRegexp() ([]byte, error) {
	pattern, _, err := w.deserializeStringData()
	if err != nil {
		return nil, fmt.Errorf("deserializing pattern in regexp: %w", err)
	}
	flags, _, err := w.deserializeStringData()
	if err != nil {
		return nil, fmt.Errorf("deserializing flags in regexp: %w", err)
	}

	regexFull := append([]byte("/"), pattern...)
	regexFull = append(regexFull, []byte("/")...)
	regexFull = append(regexFull, flags...)

	return regexFull, nil
}

var errorEnumToStringMap = map[uint8][]byte{
	errorTypeError:          []byte("Error"),
	errorTypeEvalError:      []byte("EvalError"),
	errorTypeRangeError:     []byte("RangeError"),
	errorTypeReferenceError: []byte("ReferenceError"),
	errorTypeSyntaxError:    []byte("SyntaxError"),
	errorTypeTypeError:      []byte("TypeError"),
	errorTypeURIError:       []byte("URIError"),
}

// deserializeError handles the upcoming error object.
// - uint8: error type
// - nullable string: error message
// - Number: line
// - Number: column
// - nullable string: source url
// - nullable string: stack
// - nullable string: cause
// See https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Error
// for documentation of property names and types.
func (w *webkitDeserializer) deserializeError() ([]byte, error) {
	// Create a map to hold the error properties
	errorObj := map[string]string{
		"name": "Error",
	}

	// Read type
	errorTypeEnum, err := w.reader.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("reading error type: %w", err)
	}
	errorTypeStr, ok := errorEnumToStringMap[errorTypeEnum]
	if !ok {
		return nil, fmt.Errorf("unknown error type %d", errorTypeEnum)
	}
	errorObj["type"] = string(errorTypeStr)

	// Read message
	msg, err := w.deserializeNullableString()
	if err != nil {
		return nil, fmt.Errorf("reading error message: %w", err)
	}
	errorObj["message"] = string(msg)

	// Read line
	var line uint32
	if err := binary.Read(w.reader, binary.LittleEndian, &line); err != nil {
		return nil, fmt.Errorf("reading error line: %w", err)
	}
	errorObj["lineNumber"] = strconv.Itoa(int(line))

	// Read column
	var column uint32
	if err := binary.Read(w.reader, binary.LittleEndian, &column); err != nil {
		return nil, fmt.Errorf("reading error column: %w", err)
	}
	errorObj["column"] = strconv.Itoa(int(column))

	// Read source URL
	sourceUrl, err := w.deserializeNullableString()
	if err != nil {
		return nil, fmt.Errorf("reading error source url: %w", err)
	}
	errorObj["fileName"] = string(sourceUrl)

	// Read stack
	stack, err := w.deserializeNullableString()
	if err != nil {
		return nil, fmt.Errorf("reading error stack: %w", err)
	}
	errorObj["stack"] = string(stack)

	// Read cause
	cause, err := w.deserializeNullableString()
	if err != nil {
		return nil, fmt.Errorf("reading error cause: %w", err)
	}
	errorObj["cause"] = string(cause)

	// Serialize the error object to JSON
	resultBytes, err := json.Marshal(errorObj)
	if err != nil {
		return nil, fmt.Errorf("marshalling error object: %w", err)
	}

	return resultBytes, nil
}

// deserializeNullableString handles the upcoming nullable string, which is one byte indicating
// whether the string is null, and then if it's not null, a StringData.
func (w *webkitDeserializer) deserializeNullableString() ([]byte, error) {
	isNull, err := w.reader.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("reading isNull: %w", err)
	}
	if isNull == 1 {
		return nil, nil
	}
	str, _, err := w.deserializeStringData()
	if err != nil {
		return nil, fmt.Errorf("reading string data in non-null nullable string: %w", err)
	}
	return str, nil
}
