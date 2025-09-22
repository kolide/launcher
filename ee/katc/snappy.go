package katc

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/golang/snappy"
	"github.com/kolide/launcher/ee/observability"
)

// snappyDecode is a dataProcessingStep that decodes data compressed with snappy.
// We use this to decode data retrieved from Firefox IndexedDB sqlite-backed databases.
func snappyDecode(ctx context.Context, _ *slog.Logger, row map[string][]byte) (map[string][]byte, error) {
	_, span := observability.StartSpan(ctx)
	defer span.End()

	decodedRow := make(map[string][]byte)

	for k, v := range row {
		decodedResultBytes, err := snappy.Decode(nil, v)
		if err != nil {
			return nil, fmt.Errorf("decoding data for key %s: %w", k, err)
		}

		decodedRow[k] = decodedResultBytes
	}

	return decodedRow, nil
}
