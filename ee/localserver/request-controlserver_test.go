package localserver

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kolide/launcher/ee/localserver/mocks"
	"github.com/stretchr/testify/require"
)

func Test_localServer_requestControlServerFetch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		expectedCode        int
		controlServerReturn error
	}{
		{
			name:         "happy path",
			expectedCode: http.StatusOK,
		},
		{
			name:                "err",
			expectedCode:        http.StatusBadRequest,
			controlServerReturn: errors.New("error"),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockControlServer := mocks.NewControlServer(t)
			mockControlServer.On("Fetch").Return(tt.controlServerReturn).Once()

			var logBytes bytes.Buffer
			server := testServer(t, &logBytes)
			server.controlServer = mockControlServer

			req, err := http.NewRequest("", "", nil)
			require.NoError(t, err)

			handler := http.HandlerFunc(server.requestControlServerFetchFunc)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			require.Equal(t, tt.expectedCode, rr.Code)
		})
	}
}
