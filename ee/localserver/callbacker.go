package localserver

// callbacker is a somewhat convoluted set of routines that allows the _inner_ handlers of krytobox
// to read a callback URL, and pass it back to the outer middleware, which can then trigger an appropriate
// callback. This allows the callback URl to be part of the signed encryption.

import (
	"context"
	"fmt"
	"net/http"
)

type contextKey int

const (
	postbackKey contextKey = iota
)

type callbacker struct {
	url string
}

func (cb *callbacker) Send(data string) {
	if cb.url == "" {
		fmt.Println("No callback")
		return
	}

	fmt.Println("Calling back to: ", cb.url)
}

func (cb *callbacker) PostURL(url string) {
	cb.url = url
}

func CallbackWrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if next == nil {
			return
		}

		// need a different cb for each request.
		cb := &callbacker{}

		// Setup a context with our callbacker
		newctx := context.WithValue(r.Context(), postbackKey, cb)

		// Pass the request along
		next.ServeHTTP(w, r.WithContext(newctx))

		// Trigger the callback
		cb.Send("data not implemented")
	})
}

// Called from the inner middleware
func SetPostbackViaContext(ctx context.Context, url string) error {
	fromctx := ctx.Value(postbackKey)
	if fromctx == nil {
		return nil
	}

	cb, ok := fromctx.(*callbacker)
	if !ok {
		return fmt.Errorf("wrong type in callback, got: %v", cb)
	}

	cb.PostURL(url)

	return nil
}
