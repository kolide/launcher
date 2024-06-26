package katc

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/golang/snappy"
)

// snappyDecode is a dataProcessingStep that decodes data compressed with snappy
func snappyDecode(ctx context.Context, data []byte, slogger *slog.Logger) ([]byte, error) {
	decodedResultBytes, err := snappy.Decode(nil, data)
	if err != nil {
		return nil, fmt.Errorf("decoding column: %w", err)
	}

	return decodedResultBytes, nil
}
