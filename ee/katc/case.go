package katc

import (
	"context"
	"log/slog"

	"github.com/kolide/launcher/ee/observability"
	"github.com/serenize/snaker"
)

func camelToSnake(ctx context.Context, _ *slog.Logger, row map[string][]byte) (map[string][]byte, error) {
	_, span := observability.StartSpan(ctx)
	defer span.End()

	snakeCaseRow := make(map[string][]byte)
	for k, v := range row {
		snakeCaseKey := snaker.CamelToSnake(k)
		snakeCaseRow[snakeCaseKey] = v
	}

	return snakeCaseRow, nil
}
