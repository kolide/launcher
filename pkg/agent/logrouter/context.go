package logrouter

import (
	"context"

	"github.com/go-kit/kit/log"
)

type key int

const logrouterKey key = 0

func NewCtx(ctx context.Context, lr *logRouter) context.Context {
	return context.WithValue(ctx, logrouterKey, lr)
}

func FromContext(ctx context.Context) *logRouter {
	v, ok := ctx.Value(logrouterKey).(*logRouter)
	if !ok {
		lr, err := New(log.NewNopLogger())
		if err != nil {
			panic(err)
		}

		return lr
	}

	return v
}
