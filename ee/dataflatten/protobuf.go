package dataflatten

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"unicode/utf8"

	"google.golang.org/protobuf/encoding/protowire"
)

// ProtobufFile reads a marshaled protobuf file and returns flattened rows.
// Because protobuf is a schema-less binary format at the wire level,
// field numbers are used as keys (e.g., "1", "2", "3") and types are
// inferred heuristically from the wire format.
func ProtobufFile(file string, opts ...FlattenOpts) ([]Row, error) {
	rawdata, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return Protobuf(rawdata, opts...)
}

// Protobuf decodes raw protobuf wire-format data and returns flattened rows.
func Protobuf(rawdata []byte, opts ...FlattenOpts) ([]Row, error) {
	data, err := decodeRawProtobuf(rawdata)
	if err != nil {
		return nil, fmt.Errorf("decoding protobuf: %w", err)
	}
	return Flatten(data, opts...)
}

// decodeRawProtobuf parses protobuf wire-format bytes into a map keyed
// by field number strings. Repeated fields become []any slices.
func decodeRawProtobuf(data []byte) (map[string]any, error) {
	if len(data) == 0 {
		return make(map[string]any), nil
	}

	fields := make(map[string][]any)

	for len(data) > 0 {
		num, wtype, tagLen := protowire.ConsumeTag(data)
		if tagLen < 0 {
			return nil, fmt.Errorf("invalid protobuf tag")
		}
		if num > protowire.MaxValidNumber {
			return nil, fmt.Errorf("field number %d exceeds maximum valid number", num)
		}
		data = data[tagLen:]

		key := strconv.FormatInt(int64(num), 10)
		var val any

		switch wtype {
		case protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return nil, fmt.Errorf("invalid varint for field %s", key)
			}
			data = data[n:]
			val = v

		case protowire.Fixed64Type:
			v, n := protowire.ConsumeFixed64(data)
			if n < 0 {
				return nil, fmt.Errorf("invalid fixed64 for field %s", key)
			}
			data = data[n:]
			val = v

		case protowire.Fixed32Type:
			v, n := protowire.ConsumeFixed32(data)
			if n < 0 {
				return nil, fmt.Errorf("invalid fixed32 for field %s", key)
			}
			data = data[n:]
			val = v

		case protowire.BytesType:
			v, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return nil, fmt.Errorf("invalid length-delimited field %s", key)
			}
			data = data[n:]
			val = decodeBytesField(v)

		case protowire.StartGroupType:
			v, n := protowire.ConsumeGroup(num, data)
			if n < 0 {
				return nil, fmt.Errorf("invalid group for field %s", key)
			}
			data = data[n:]
			nested, err := decodeRawProtobuf(v)
			if err != nil {
				return nil, fmt.Errorf("decoding group field %s: %w", key, err)
			}
			val = nested

		default:
			return nil, fmt.Errorf("unknown wire type %d for field %s", wtype, key)
		}

		fields[key] = append(fields[key], val)
	}

	result := make(map[string]any, len(fields))
	for k, vals := range fields {
		if len(vals) == 1 {
			result[k] = vals[0]
		} else {
			result[k] = vals
		}
	}

	return result, nil
}

// decodeBytesField interprets a length-delimited protobuf field.
// Printable UTF-8 strings are returned directly. Non-printable data
// is tried as a nested protobuf message. Remaining binary data is
// base64-encoded.
func decodeBytesField(data []byte) any {
	if len(data) == 0 {
		return ""
	}

	// Human-readable text is almost certainly a string field, not a nested message.
	if pbBytesLooksLikeString(data) {
		return string(data)
	}

	if nested, err := decodeRawProtobuf(data); err == nil && len(nested) > 0 {
		return nested
	}

	if utf8.Valid(data) {
		return string(data)
	}

	return base64.StdEncoding.EncodeToString(data)
}

// pbBytesLooksLikeString reports whether data looks like a human-readable
// string: valid UTF-8 with no control characters except tab, newline, and
// carriage return.
func pbBytesLooksLikeString(data []byte) bool {
	if !utf8.Valid(data) {
		return false
	}
	for _, b := range data {
		if b < 0x20 && b != '\t' && b != '\n' && b != '\r' {
			return false
		}
	}
	return true
}
