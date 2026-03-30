package katc

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"

	"github.com/klauspost/compress/zstd"
	"github.com/kolide/launcher/v2/ee/observability"
)

const zstdMagicNumber uint32 = 0xFD2FB528

// zstdDecode is a dataProcessingStep that decodes data compressed with zstd.
func zstdDecode(ctx context.Context, _ *slog.Logger, row map[string][]byte) ([]map[string][]byte, error) {
	_, span := observability.StartSpan(ctx)
	defer span.End()

	var decoder, _ = zstd.NewReader(nil, zstd.WithDecoderConcurrency(0))
	defer decoder.Close()

	decodedRow := make(map[string][]byte)
	for k, v := range row {
		// To support data sources that have both compressed and uncompressed values,
		// we check to see if this value is zstd-compressed. If it's not, we pass it through
		// untransformed.
		if !detectZstdCompressed(v) {
			decodedRow[k] = v
			continue
		}
		decodedResultBytes, err := decoder.DecodeAll(v, nil)
		if err != nil {
			return nil, fmt.Errorf("decoding data for key %s: %w", k, err)
		}

		decodedRow[k] = decodedResultBytes
	}

	return []map[string][]byte{decodedRow}, nil
}

// zstd-compressed data is made up of one or more frames. The standard frame
// starts with a `Magic_Number`, which is 0xFD2FB528 (4 bytes, little-endian format).
// If the data to be evaluated does not start with this magic number, then we can assume
// that the data is not zstd compressed.
// See: https://datatracker.ietf.org/doc/html/rfc8878
func detectZstdCompressed(data []byte) bool {
	// We need at least six bytes for a valid frame (4 for the magic number, 2 for the frame header).
	if len(data) < 6 {
		return false
	}
	return binary.LittleEndian.Uint32(data[0:4]) == zstdMagicNumber
}
