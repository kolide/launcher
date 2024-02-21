//go:build !linux
// +build !linux

package fscrypt_info

import (
	"context"
	"errors"
	"log/slog"

	"github.com/osquery/osquery-go/plugin/table"
)

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	t.slogger.Log(ctx, slog.LevelInfo,
		"table only supported on linux",
	)
	return nil, errors.New("Platform Unsupported")
}
