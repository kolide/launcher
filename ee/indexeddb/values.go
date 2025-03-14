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
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/kolide/launcher/pkg/traces"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// See: https://github.com/v8/v8/blob/master/src/objects/value-serializer.cc
const (
	tokenVersion             byte = 0xff // in header
	tokenPadding             byte = 0x00
	tokenVerifyObjectCount   byte = 0x3f // ?
	tokenTheHole             byte = 0x2d // -
	tokenUndefined           byte = 0x5f // _
	tokenNull                byte = 0x30
	tokenTrue                byte = 0x54 // T
	tokenFalse               byte = 0x46 // F
	tokenInt32               byte = 0x49 // I
	tokenUint32              byte = 0x55 // U
	tokenDouble              byte = 0x4e // N
	tokenBigInt              byte = 0x5a // Z
	tokenUtf8Str             byte = 0x53 // S
	tokenAsciiStr            byte = 0x22 // "
	tokenUtf16Str            byte = 0x63 // c
	tokenObjReference        byte = 0x5e // ^
	tokenObjectBegin         byte = 0x6f // o
	tokenObjectEnd           byte = 0x7b // {
	tokenBeginSparseArray    byte = 0x61 // a
	tokenEndSparseArray      byte = 0x40 // @
	tokenBeginDenseArray     byte = 0x41 // A
	tokenEndDenseArray       byte = 0x24 // $
	tokenDate                byte = 0x44 // D
	tokenTrueObj             byte = 0x79 // y
	tokenFalseObj            byte = 0x78 // x
	tokenNumberObj           byte = 0x6e // n
	tokenBigIntObj           byte = 0x7a // z
	tokenStringObj           byte = 0x73 // s
	tokenRegexp              byte = 0x52 // R
	tokenMapBegin            byte = 0x3b // ;
	tokenMapEnd              byte = 0x3a // :
	tokenSetBegin            byte = 0x27 // '
	tokenSetEnd              byte = 0x2c // ,
	tokenArrayBuffer         byte = 0x42 // B
	tokenArrayBufferTransfer byte = 0x74 // t
	tokenArrayBufferView     byte = 0x56 // V
	tokenSharedArrayBuffer   byte = 0x75 // u
	tokenWasmModuleTransfer  byte = 0x77 // w
	tokenHostObj             byte = 0x5c // /
	tokenWasmMemoryTransfer  byte = 0x6d // m
	tokenError               byte = 0x72 // r

	// The name of these consts is a guess based on the context where I've seen 0x03 and 0x01 pop up.
	// They may signal something besides array termination.
	tokenPossiblyArrayTermination0x03 byte = 0x03
	tokenPossiblyArrayTermination0x01 byte = 0x01
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
			return obj, fmt.Errorf("object property name has unexpected non-string type %02x / `%s`", objPropertyStart, string(objPropertyStart))
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
		case tokenUtf8Str, tokenAsciiStr:
			return deserializeAsciiStr(srcReader)
		case tokenUtf16Str:
			return deserializeUtf16Str(srcReader)
		case tokenStringObj:
			return deserializeStringObject(srcReader)
		case tokenRegexp:
			return deserializeRegexp(srcReader)
		case tokenTrue, tokenTrueObj:
			return []byte("true"), nil
		case tokenFalse, tokenFalseObj:
			return []byte("false"), nil
		case tokenUndefined, tokenNull:
			return nil, nil
		case tokenInt32:
			propertyInt, err := binary.ReadVarint(srcReader)
			if err != nil {
				return nil, fmt.Errorf("decoding int32: %w", err)
			}
			return []byte(strconv.Itoa(int(propertyInt))), nil
		case tokenDouble, tokenNumberObj:
			var d float64
			if err := binary.Read(srcReader, binary.NativeEndian, &d); err != nil {
				return nil, fmt.Errorf("decoding double: %w", err)
			}
			return []byte(strconv.FormatFloat(d, 'f', -1, 64)), nil
		case tokenBigInt, tokenBigIntObj:
			return deserializeBigInt(srcReader)
		case tokenDate:
			var d float64
			if err := binary.Read(srcReader, binary.NativeEndian, &d); err != nil {
				return nil, fmt.Errorf("decoding double as date: %w", err)
			}
			// d is milliseconds since epoch
			return []byte(time.UnixMilli(int64(d)).UTC().String()), nil
		case tokenBeginSparseArray:
			return deserializeSparseArray(ctx, slogger, srcReader)
		case tokenBeginDenseArray:
			return deserializeDenseArray(ctx, slogger, srcReader)
		case tokenMapBegin:
			return deserializeMap(ctx, slogger, srcReader)
		case tokenSetBegin:
			return deserializeSet(ctx, slogger, srcReader)
		case tokenPadding, tokenVerifyObjectCount, tokenTheHole:
			// We don't care about these types -- we want to try reading again
			var err error
			nextToken, err = nextNonPaddingByte(srcReader)
			if err != nil {
				return nil, fmt.Errorf("reading next non-padding byte after padding byte: %w", err)
			}
			continue
		case tokenArrayBuffer:
			return deserializeArrayBuffer(ctx, slogger, srcReader)
		case tokenObjReference:
			// This is a reference to an already-deserialized object. For now, we don't
			// really care which one -- we just want to continue parsing. Get its ID and
			// return that.
			objectId, err := binary.ReadUvarint(srcReader)
			if err != nil {
				return nil, fmt.Errorf("reading id of object: %w", err)
			}
			return []byte(fmt.Sprintf("object id %d", objectId)), nil
		case tokenArrayBufferView:
			return deserializePresumablyEmptyArrayBufferView(srcReader)
		case tokenArrayBufferTransfer:
			return nil, errors.New("deserialization not implemented for array buffer transfers")
		case tokenSharedArrayBuffer:
			return nil, errors.New("deserialization not implemented for shared ArrayBuffer")
		case tokenWasmMemoryTransfer, tokenWasmModuleTransfer:
			return nil, errors.New("deserialization not implemented for wasm transfers")
		case tokenError:
			return nil, errors.New("deserialization not implemented for error")
		case tokenHostObj:
			return nil, errors.New("deserialization not implemented for host object")
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

// deserializeBigInt deserializes exactly as much of the upcoming BigInt as necessary
// to get to the next value. We do not actually convert the raw digits to a string,
// since that is proving to be a lot of work -- we just return a placeholder string.
// We can revisit this decision once we determine we actually care about any BigInt values.
func deserializeBigInt(srcReader *bytes.Reader) ([]byte, error) {
	// First up -- read the bitfield. It's a uint32.
	bitfield, err := binary.ReadUvarint(srcReader)
	if err != nil {
		return nil, fmt.Errorf("reading bitfield for BigInt: %w", err)
	}

	// Use the bitfield to determine a) the sign for this bigint and b) the number of bytes
	// used to store this bigint. The sign is the last bit, and the length is the remainder.
	isNegative := bitfield & 1
	bigIntLen := bitfield & ((1 << 32) - 1)
	numBytesToRead := bigIntLen / 2

	// Read the next bigIntLenInBytes bytes
	bigIntRawBytes := make([]byte, numBytesToRead)
	for i := 0; i < int(numBytesToRead); i++ {
		b, err := srcReader.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("reading byte %d of %d for BigInt: %w", i, numBytesToRead, err)
		}
		bigIntRawBytes[i] = b
	}

	// Return a placeholder string
	if isNegative > 0 {
		return []byte("-?n"), nil
	}
	return []byte("?n"), nil
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
		case tokenPossiblyArrayTermination0x01, tokenPossiblyArrayTermination0x03:
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
		arrayItem, err := deserializeNext(ctx, slogger, nextByte, srcReader)
		if err != nil {
			return nil, fmt.Errorf("decoding next item in dense array: %w", err)
		}
		arrItems[i] = string(arrayItem) // cast to string so it's readable when marshalled again below
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
	arrItems := make([]any, arrayLen)
	for i := 0; i < int(arrayLen); i++ {
		// Read next token to see if the array is completed
		nextByte, err := srcReader.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("reading next byte: %w", err)
		}
		// Array item! Unread the byte
		arrayItem, err := deserializeNext(ctx, slogger, nextByte, srcReader)
		if err != nil {
			return nil, fmt.Errorf("decoding next item in dense array: %w", err)
		}
		arrItems[i] = string(arrayItem) // cast to string so it's readable when marshalled again below
	}

	// At the end of the array we have some padding and additional data -- consume
	// that data
	reachedEndOfArray := false
	for {
		if reachedEndOfArray {
			break
		}

		nextByte, err := srcReader.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("reading next byte at end of dense array: %w", err)
		}

		switch nextByte {
		case tokenEndDenseArray:
			// We have extra padding here -- the next two bytes are `properties_written` and `length`,
			// respectively. We don't care about checking them, so we read and discard them.
			_, _ = srcReader.ReadByte()
			_, _ = srcReader.ReadByte()
			reachedEndOfArray = true
			continue
		case tokenPossiblyArrayTermination0x01, tokenPossiblyArrayTermination0x03:
			// This occurs immediately before tokenEndSparseArray -- not sure why. We can ignore it.
			continue
		default:
			return nil, fmt.Errorf("unexpected byte at end of dense array %02x / `%s`", nextByte, string(nextByte))
		}
	}

	arrBytes, err := json.Marshal(arrItems)
	if err != nil {
		return nil, fmt.Errorf("marshalling array: %w", err)
	}

	return arrBytes, nil
}

// Represent values from enum class ArrayBufferViewTag
const (
	arrayBufferViewTagInt8Array         byte = 0x62 // b
	arrayBufferViewTagUint8Array        byte = 0x42 // B
	arrayBufferViewTagUint8ClampedArray byte = 0x43 // C
	arrayBufferViewTagInt16Array        byte = 0x77 // w
	arrayBufferViewTagUint16Array       byte = 0x57 // W
	arrayBufferViewTagInt32Array        byte = 0x64 // d
	arrayBufferViewTagUint32Array       byte = 0x44 // D
	arrayBufferViewTagFloat32Array      byte = 0x66 // f
	arrayBufferViewTagFloat64Array      byte = 0x46 // F
	arrayBufferViewTagBigInt64Array     byte = 0x71 // q
	arrayBufferViewTagBigUint64Array    byte = 0x51 // Q
	arrayBufferViewTagDataView          byte = 0x3f // ?
)

func deserializeArrayBuffer(ctx context.Context, slogger *slog.Logger, srcReader *bytes.Reader) ([]byte, error) {
	// Next up is the raw length of the array buffer -- read that, then read in the raw data
	arrayBufLen, err := binary.ReadUvarint(srcReader)
	if err != nil {
		return nil, fmt.Errorf("reading uvarint as ArrayBuffer length: %w", err)
	}
	rawArrayBufferData := make([]byte, arrayBufLen)
	for i := 0; i < int(arrayBufLen); i++ {
		nextByte, err := srcReader.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("reading byte %d of %d in ArrayBuffer: %w", i, arrayBufLen, err)
		}
		rawArrayBufferData[i] = nextByte
	}

	// After the ArrayBuffer may come the view -- check for that next
	nextByte, err := srcReader.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("peeking byte after ArrayBuffer: %w", err)
	}
	if nextByte != tokenArrayBufferView {
		// Not a view next -- the ArrayBuffer is standalone. Unread the byte and return the raw data.
		if err := srcReader.UnreadByte(); err != nil {
			return nil, fmt.Errorf("unreading byte after peeking ahead post-ArrayBuffer: %w", err)
		}

		return rawArrayBufferData, nil
	}

	// Now the view! The view effectively "consumes" the ArrayBuffer that came before it
	// by telling us how to interpret the raw data. The next values to read the subtag
	// ArrayBufferViewTag, then two uint32s: the byte offset and the byte length.
	arrayBufferViewTag, err := srcReader.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("reading array buffer view tag: %w", err)
	}
	byteOffset, err := binary.ReadUvarint(srcReader)
	if err != nil {
		return nil, fmt.Errorf("reading byte offset for ArrayBuffer view: %w", err)
	}
	byteLength, err := binary.ReadUvarint(srcReader)
	if err != nil {
		return nil, fmt.Errorf("reading byte length for ArrayBuffer view: %w", err)
	}

	// Handle the only non-TypedArray case
	if arrayBufferViewTag == arrayBufferViewTagDataView {
		// We're not implementing data views right now -- return the raw data
		slogger.Log(ctx, slog.LevelWarn,
			"data view parsing not implemented for array buffer view, returning raw data instead",
		)
		return rawArrayBufferData, nil
	}

	// Handle auto-length TypedArrays. This is the only case where byteLength does not match len(rawArrayBufferData).
	if byteLength == math.MaxUint64 {
		// We're not implementing auto-length TypedArrays right now -- return the raw data.
		slogger.Log(ctx, slog.LevelWarn,
			"parsing not implemented for auto-length TypedArrays, returning raw data instead",
		)
		return rawArrayBufferData, nil
	}

	// Create reader to re-interpret TypedArray data
	typedArrayReader := bytes.NewReader(rawArrayBufferData)

	// Read through padding, if any
	for i := 0; i < int(byteOffset); i++ {
		if _, err := typedArrayReader.ReadByte(); err != nil {
			return nil, fmt.Errorf("reading byte %d of %d in padding: %w", i, byteOffset, err)
		}
	}

	// Reinterpret TypedArray data as appropriate type
	var result any
	switch arrayBufferViewTag {
	case arrayBufferViewTagInt8Array, arrayBufferViewTagUint8Array, arrayBufferViewTagUint8ClampedArray:
		result, err = readUint8Array(typedArrayReader)
	case arrayBufferViewTagInt16Array, arrayBufferViewTagUint16Array:
		result, err = readUint16Array(typedArrayReader)
	case arrayBufferViewTagInt32Array, arrayBufferViewTagUint32Array:
		result, err = readUint32Array(typedArrayReader)
	case arrayBufferViewTagFloat32Array:
		result, err = readFloat32Array(typedArrayReader)
	case arrayBufferViewTagBigInt64Array, arrayBufferViewTagBigUint64Array:
		result, err = readUint64Array(typedArrayReader)
	case arrayBufferViewTagFloat64Array:
		result, err = readFloat64Array(typedArrayReader)
	default:
		return nil, fmt.Errorf("unsupported TypedArray type %s", string(arrayBufferViewTag))
	}
	if err != nil {
		return nil, fmt.Errorf("reading TypedArray of type %d: %w", arrayBufferViewTag, err)
	}

	arrBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshalling TypedArray of type %s: %w", string(arrayBufferViewTag), err)
	}

	return arrBytes, nil
}

// readUint8Array is suitable for reading TypedArrays: Uint8Array, Int8Array, Uint8ClampedArray
func readUint8Array(typedArrayReader *bytes.Reader) ([]uint8, error) {
	// rawArrayLength is the same as actual array length for uint8
	arrayLen := typedArrayReader.Len()
	result := make([]uint8, arrayLen)
	for i := 0; i < arrayLen; i++ {
		if err := binary.Read(typedArrayReader, binary.NativeEndian, &result[i]); err != nil {
			return nil, fmt.Errorf("reading uint8 at index %d in TypedArray: %w", i, err)
		}
	}
	return result, nil
}

// readUint16Array is suitable for reading TypedArrays: Uint16Array, Int16Array
func readUint16Array(typedArrayReader *bytes.Reader) ([]uint16, error) {
	arrayLen := typedArrayReader.Len() / 2
	result := make([]uint16, arrayLen)
	for i := 0; i < arrayLen; i++ {
		if err := binary.Read(typedArrayReader, binary.NativeEndian, &result[i]); err != nil {
			return nil, fmt.Errorf("reading uint16 at index %d in TypedArray: %w", i, err)
		}
	}
	return result, nil
}

// readUint32Array is suitable for reading TypedArrays: Uint32Array, Int32Array
func readUint32Array(typedArrayReader *bytes.Reader) ([]uint32, error) {
	arrayLen := typedArrayReader.Len() / 4
	result := make([]uint32, arrayLen)
	for i := 0; i < arrayLen; i++ {
		if err := binary.Read(typedArrayReader, binary.NativeEndian, &result[i]); err != nil {
			return nil, fmt.Errorf("reading uint32 at index %d in TypedArray: %w", i, err)
		}
	}
	return result, nil
}

// readFloat32Array is suitable for reading TypedArrays: Float32Array
func readFloat32Array(typedArrayReader *bytes.Reader) ([]float32, error) {
	arrayLen := typedArrayReader.Len() / 4
	result := make([]float32, arrayLen)
	for i := 0; i < arrayLen; i++ {
		if err := binary.Read(typedArrayReader, binary.NativeEndian, &result[i]); err != nil {
			return nil, fmt.Errorf("reading float32 at index %d in TypedArray: %w", i, err)
		}
	}
	return result, nil
}

// readUint64Array is suitable for reading TypedArrays: BigUint64Array, BigInt64Array
func readUint64Array(typedArrayReader *bytes.Reader) ([]uint64, error) {
	arrayLen := typedArrayReader.Len() / 8
	result := make([]uint64, arrayLen)
	for i := 0; i < arrayLen; i++ {
		if err := binary.Read(typedArrayReader, binary.NativeEndian, &result[i]); err != nil {
			return nil, fmt.Errorf("reading uint64 at index %d in TypedArray: %w", i, err)
		}
	}
	return result, nil
}

// readFloat64Array is suitable for reading TypedArrays: Float64Array
func readFloat64Array(typedArrayReader *bytes.Reader) ([]float64, error) {
	arrayLen := typedArrayReader.Len() / 8
	result := make([]float64, arrayLen)
	for i := 0; i < arrayLen; i++ {
		if err := binary.Read(typedArrayReader, binary.NativeEndian, &result[i]); err != nil {
			return nil, fmt.Errorf("reading float64 at index %d in TypedArray: %w", i, err)
		}
	}
	return result, nil
}

// deserializePresumablyEmptyArrayBufferView handles the case where we have an array buffer view
// with no data in it. Array buffer views typically always follow an ArrayBuffer, and they consume
// that data. However, when there is no data, sometimes they appear standalone. In that case,
// we confirm that the view is standalone, and return an empty array.
func deserializePresumablyEmptyArrayBufferView(srcReader *bytes.Reader) ([]byte, error) {
	// An array buffer view starts with its subtag (indicating type), byte offset,
	// and byte length. We only really care about the byte length here, since
	// we are expecting a totally empty array.
	if _, err := srcReader.ReadByte(); err != nil {
		return nil, fmt.Errorf("reading array buffer view tag: %w", err)
	}
	if _, err := binary.ReadUvarint(srcReader); err != nil {
		return nil, fmt.Errorf("reading byte offset for ArrayBuffer view: %w", err)
	}
	byteLength, err := binary.ReadUvarint(srcReader)
	if err != nil {
		return nil, fmt.Errorf("reading byte length for ArrayBuffer view: %w", err)
	}

	// Check to make sure the length is 0, like we expect
	if byteLength != 0 {
		return nil, fmt.Errorf("found standalone array buffer view with length %d despite no preceding data", byteLength)
	}

	// Next, we typically see an 0x03, which we interpret as an array termination byte.
	nextByte, err := srcReader.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("reading next byte at end of dense array: %w", err)
	}
	if nextByte != tokenPossiblyArrayTermination0x03 {
		return nil, fmt.Errorf("unexpected byte at end of standalone array buffer view: %02x / `%s`", nextByte, string(nextByte))
	}

	return []byte("[]"), nil
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

// deserializeStringObject handles the upcoming String in srcReader.
func deserializeStringObject(srcReader *bytes.Reader) ([]byte, error) {
	stringTypeToken, err := nextNonPaddingByte(srcReader)
	if err != nil {
		return nil, fmt.Errorf("reading token to determine encoding for String: %w", err)
	}

	switch stringTypeToken {
	case tokenAsciiStr, tokenUtf8Str:
		return deserializeAsciiStr(srcReader)
	case tokenUtf16Str:
		return deserializeUtf16Str(srcReader)
	default:
		return nil, fmt.Errorf("unknown token for String %02x / `%s`", stringTypeToken, string(stringTypeToken))
	}
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
