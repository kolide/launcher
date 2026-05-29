package katc

import (
	"context"
	"log/slog"

	"github.com/kolide/launcher/v2/ee/observability"
	"github.com/kolide/launcher/v2/pkg/snake"
)

func camelToSnake(ctx context.Context, _ *slog.Logger, row map[string][]byte) ([]map[string][]byte, error) {
	_, span := observability.StartSpan(ctx)
	defer span.End()

	snakeCaseRow := make(map[string][]byte)
	for k, v := range row {
		snakeCaseKey := snake.CamelToSnake(k)
		snakeCaseRow[snakeCaseKey] = v
	}

	return []map[string][]byte{snakeCaseRow}, nil
}
