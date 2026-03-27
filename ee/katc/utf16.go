package katc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// utf16Decode is a rowTransformStep that decodes UTF-16 encoded data.
// In practice we're not seeing all key/values being consistently UTF-16 encoded
// (e.g. some values are, but keys and some other values are not)
// so utf16DecodeValue uses some heuristics to determine whether to apply the transformation.
func utf16Decode(_ context.Context, _ *slog.Logger, row map[string][]byte) (map[string][]byte, error) {
	decodedRow := make(map[string][]byte)
	for k, v := range row {
		decoded, err := utf16DecodeValue(v)
		if err != nil {
			return nil, fmt.Errorf("decoding UTF-16 for key %s: %w", k, err)
		}
		decodedRow[k] = decoded
	}
	return decodedRow, nil
}

// utf16DecodeValue selects the decoding based on byte order marks and early byte sequences:
//   - BOM 0xFF 0xFE: UTF-16 little endian
//   - BOM 0xFE 0xFF: UTF-16 big endian
//   - First byte is null: UTF-16 big endian
//   - Second byte is null: UTF-16 little endian
//   - Otherwise: return data as-is
func utf16DecodeValue(v []byte) ([]byte, error) {
	if len(v) < 2 {
		return v, nil
	}

	var decoder transform.Transformer
	switch {
	case v[0] == 0xFF && v[1] == 0xFE:
		decoder = unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder()
	case v[0] == 0xFE && v[1] == 0xFF:
		decoder = unicode.UTF16(unicode.BigEndian, unicode.UseBOM).NewDecoder()
	case v[0] == 0x00:
		decoder = unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM).NewDecoder()
	case v[1] == 0x00:
		decoder = unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewDecoder()
	default:
		return v, nil
	}

	utf16Reader := transform.NewReader(bytes.NewReader(v), decoder)
	return io.ReadAll(utf16Reader)
}
