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

func Test_localServer_requestControlServiceFetch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		expectedCode         int
		controlServiceReturn error
	}{
		{
			name:         "happy path",
			expectedCode: http.StatusOK,
		},
		{
			name:                 "err",
			expectedCode:         http.StatusBadRequest,
			controlServiceReturn: errors.New("error"),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockControlService := mocks.NewControlService(t)
			mockControlService.On("Fetch").Return(tt.controlServiceReturn).Once()

			var logBytes bytes.Buffer
			server := testServer(t, &logBytes)
			server.controlService = mockControlService

			req, err := http.NewRequest("", "", nil)
			require.NoError(t, err)

			handler := http.HandlerFunc(server.requestControlSericeFetchFunc)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			require.Equal(t, tt.expectedCode, rr.Code)
		})
	}
}
