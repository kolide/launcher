package katc

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"

	"github.com/kolide/launcher/ee/observability"
)

// hexDecode is a dataProcessingStep that decodes data that is hex-encoded.
func hexDecode(ctx context.Context, _ *slog.Logger, row map[string][]byte) (map[string][]byte, error) {
	_, span := observability.StartSpan(ctx)
	defer span.End()

	decodedRow := make(map[string][]byte)

	for k, v := range row {
		// Hex value may look like `X'<value here>'` -- remove surrounding chars if so.
		v := strings.TrimSuffix(strings.TrimPrefix(string(v), "X'"), "'")
		decodedBytes, err := hex.DecodeString(v)
		if err != nil {
			return nil, fmt.Errorf("decoding data for key %s: %w", k, err)
		}

		// Strip out null bytes
		decodedRow[k] = bytes.ReplaceAll(decodedBytes, []byte("\u0000"), []byte(""))
	}

	return decodedRow, nil
}
