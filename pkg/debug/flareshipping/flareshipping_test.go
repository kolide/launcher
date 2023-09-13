package flareshipping

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/debug/flareshipping/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRunFlareShip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		assertion assert.ErrorAssertionFunc
	}{
		{
			name:      "happy path",
			assertion: assert.NoError,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			flarer := mocks.NewFlarer(t)
			flarer.On("RunFlare", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

			testServer := httptest.NewServer(nil)
			testServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(testServer.URL))
				w.WriteHeader(http.StatusOK)
			})

			tt.assertion(t, RunFlareShip(log.NewNopLogger(), nil, flarer, testServer.URL))
		})
	}
}
