package katc

import (
	"context"
	"log/slog"

	"github.com/serenize/snaker"
)

func camelToSnake(_ context.Context, _ *slog.Logger, _ string, row map[string][]byte) (map[string][]byte, error) {
	snakeCaseRow := make(map[string][]byte)
	for k, v := range row {
		snakeCaseKey := snaker.CamelToSnake(k)
		snakeCaseRow[snakeCaseKey] = v
	}

	return snakeCaseRow, nil
}
