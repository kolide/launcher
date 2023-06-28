package logshipper

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/kolide/kit/ulid"
	"github.com/stretchr/testify/require"
)

func Test_authedHttpSender_Send(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
	}{
		{
			name: "happy path",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dataToSend := []byte(ulid.New())
			token := ulid.New()

			wg := sync.WaitGroup{}
			wg.Add(1)
			// create http test server with handle func that returns 200 OK
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, token, r.Header.Get("authorization"))
				w.WriteHeader(http.StatusOK)
				wg.Done()
			}))
			defer ts.Close()

			authedSender := newAuthHttpSender()
			authedSender.endpoint = ts.URL
			authedSender.authtoken = token
			authedSender.Send(bytes.NewBuffer(dataToSend))

			wg.Wait()
		})
	}
}
