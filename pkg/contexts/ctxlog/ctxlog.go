package ctxlog

import (
	"context"

	"github.com/go-kit/kit/log"
)

type key int

const loggerKey key = 0

func NewContext(ctx context.Context, logger log.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

func FromContext(ctx context.Context) log.Logger {
	v, ok := ctx.Value(loggerKey).(log.Logger)
	if !ok {
		return log.NewNopLogger()
	}
	return v
}
