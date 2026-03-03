package dataflatten

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"unicode/utf8"

	"github.com/bufbuild/protocompile"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
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
// If the input is valid base64, it is decoded first -- this allows binary
// protobuf data to be passed through SQL string constraints.
func Protobuf(rawdata []byte, opts ...FlattenOpts) ([]Row, error) {
	if decoded, err := base64.StdEncoding.DecodeString(string(rawdata)); err == nil {
		rawdata = decoded
	}

	data, err := decodeRawProtobuf(rawdata)
	if err != nil {
		return nil, fmt.Errorf("decoding protobuf: %w", err)
	}
	return Flatten(data, opts...)
}

// ProtobufWithSchema returns a DataFunc that decodes protobuf data using
// the provided .proto schema source and message type name. Field names from
// the schema are used as keys instead of field numbers.
func ProtobufWithSchema(protoSource []byte, messageTypeName string) DataFunc {
	return func(rawdata []byte, opts ...FlattenOpts) ([]Row, error) {
		if decoded, err := base64.StdEncoding.DecodeString(string(rawdata)); err == nil {
			rawdata = decoded
		}

		msgDesc, err := compileProtoAndFindMessage(protoSource, messageTypeName)
		if err != nil {
			return nil, fmt.Errorf("compiling proto schema: %w", err)
		}

		dynMsg := dynamicpb.NewMessage(msgDesc)
		if err := proto.Unmarshal(rawdata, dynMsg); err != nil {
			return nil, fmt.Errorf("unmarshalling protobuf with schema: %w", err)
		}

		data := dynamicMessageToMap(dynMsg)
		return Flatten(data, opts...)
	}
}

// compileProtoAndFindMessage parses a .proto source and returns the
// descriptor for the named message type.
func compileProtoAndFindMessage(protoSource []byte, messageTypeName string) (protoreflect.MessageDescriptor, error) {
	compiler := protocompile.Compiler{
		Resolver: &protocompile.SourceResolver{
			Accessor: protocompile.SourceAccessorFromMap(map[string]string{
				"input.proto": string(protoSource),
			}),
		},
	}

	files, err := compiler.Compile(context.Background(), "input.proto")
	if err != nil {
		return nil, fmt.Errorf("compiling proto: %w", err)
	}

	fd := files[0]
	msgs := fd.Messages()
	for i := 0; i < msgs.Len(); i++ {
		md := msgs.Get(i)
		if string(md.Name()) == messageTypeName || string(md.FullName()) == messageTypeName {
			return md, nil
		}
	}

	return nil, fmt.Errorf("message type %q not found in proto source", messageTypeName)
}

// dynamicMessageToMap converts a dynamicpb.Message into a map[string]any
// suitable for the Flatten function. Field names are used as keys.
func dynamicMessageToMap(msg *dynamicpb.Message) map[string]any {
	result := make(map[string]any)
	msg.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		key := string(fd.Name())
		result[key] = protoValueToAny(fd, v)
		return true
	})
	return result
}

// protoValueToAny converts a protoreflect.Value to a Go native type.
func protoValueToAny(fd protoreflect.FieldDescriptor, v protoreflect.Value) any {
	if fd.IsList() {
		list := v.List()
		items := make([]any, list.Len())
		for i := 0; i < list.Len(); i++ {
			items[i] = protoScalarToAny(fd, list.Get(i))
		}
		return items
	}

	if fd.IsMap() {
		m := v.Map()
		result := make(map[string]any)
		m.Range(func(k protoreflect.MapKey, val protoreflect.Value) bool {
			mapKey := fmt.Sprintf("%v", k.Value().Interface())
			result[mapKey] = protoScalarToAny(fd.MapValue(), val)
			return true
		})
		return result
	}

	return protoScalarToAny(fd, v)
}

func protoScalarToAny(fd protoreflect.FieldDescriptor, v protoreflect.Value) any {
	switch fd.Kind() {
	case protoreflect.BoolKind:
		return v.Bool()
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return int32(v.Int())
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return v.Int()
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return uint32(v.Uint())
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return v.Uint()
	case protoreflect.FloatKind:
		return float32(v.Float())
	case protoreflect.DoubleKind:
		return v.Float()
	case protoreflect.StringKind:
		return v.String()
	case protoreflect.BytesKind:
		b := v.Bytes()
		if utf8.Valid(b) {
			return string(b)
		}
		return base64.StdEncoding.EncodeToString(b)
	case protoreflect.EnumKind:
		enumDesc := fd.Enum().Values().ByNumber(v.Enum())
		if enumDesc != nil {
			return string(enumDesc.Name())
		}
		return int32(v.Enum())
	case protoreflect.MessageKind, protoreflect.GroupKind:
		inner, ok := v.Message().Interface().(*dynamicpb.Message)
		if ok {
			return dynamicMessageToMap(inner)
		}
		return fmt.Sprintf("%v", v.Message().Interface())
	default:
		return fmt.Sprintf("%v", v.Interface())
	}
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
