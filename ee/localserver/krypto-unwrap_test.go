package localserver

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"bytes"
	"fmt"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/krypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnwrapV0(t *testing.T) {
	t.Parallel()

	myKey, err := krypto.RsaRandomKey()
	require.NoError(t, err)

	counterpartyKey, err := krypto.RsaRandomKey()
	require.NoError(t, err)

	counterpartyPub, ok := counterpartyKey.Public().(*rsa.PublicKey)
	require.True(t, ok)

	malloryKey, err := krypto.RsaRandomKey()
	require.NoError(t, err)

	counterpartyBoxer := krypto.NewBoxer(counterpartyKey, nil)

	malloryBoxer := krypto.NewBoxer(malloryKey, nil)

	// Make some signed boxes
	signedIncorrectBox, err := counterpartyBoxer.Sign("", []byte("incorrect"))
	require.NoError(t, err)

	expectedCmd := ulid.New()
	expectedId := ulid.New()

	cmdReq := mustMarshal(t, cmdRequestType{Cmd: expectedCmd, Id: expectedId})

	signedBox, err := counterpartyBoxer.Sign("", cmdReq)
	require.NoError(t, err)

	mallorySigned, err := malloryBoxer.Sign("", cmdReq)
	require.NoError(t, err)

	var tests = []struct {
		name      string
		boxParam  string
		loggedErr string
	}{
		{
			name:      "bad base64",
			boxParam:  "This is not base64",
			loggedErr: "unable to base64 decode box",
		},

		{
			name:      "no signature",
			boxParam:  "aGVsbG8gd29ybGQK",
			loggedErr: "unable to unmarshal box",
		},

		{
			name:      "malformed cmd",
			boxParam:  base64.StdEncoding.EncodeToString(signedIncorrectBox),
			loggedErr: "unable to unmarshal cmd request",
		},

		{
			name:      "wrong signature",
			boxParam:  base64.StdEncoding.EncodeToString(mallorySigned),
			loggedErr: "unable to verify box",
		},

		{
			name:     "works",
			boxParam: base64.StdEncoding.EncodeToString(signedBox),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var logBytes bytes.Buffer

			kbm, err := NewKryptoBoxerMiddleware(log.NewLogfmtLogger(&logBytes), myKey, counterpartyPub)
			require.NoError(t, err)

			kryptoDeterminerMiddleware := NewKryptoDeterminerMiddleware(log.NewLogfmtLogger(&logBytes), kbm.UnwrapV1Hander(makeTestHandler(t)), nil)

			req := makeGetRequest(t, tt.boxParam)

			rr := httptest.NewRecorder()
			kryptoDeterminerMiddleware.ServeHTTP(rr, req)

			if tt.loggedErr != "" {
				assert.Equal(t, http.StatusUnauthorized, rr.Code)
				assert.Contains(t, logBytes.String(), tt.loggedErr)
				return
			}

			assert.Equal(t, http.StatusOK, rr.Code)
			assert.NotEmpty(t, rr.Body.String())
			assert.Equal(t, fmt.Sprintf("https://127.0.0.1:8080/%s?id=%s", expectedCmd, expectedId), rr.Body.String())
		})
	}

}

func TestMakeTestHander(t *testing.T) {
	t.Parallel()

	h := makeTestHandler(t)

	req, err := http.NewRequest("GET", "http://localhost:8080", nil)
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "http://localhost:8080", rr.Body.String())
}

func makeTestHandler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.URL.String()))
	})
}

func makeGetRequest(t *testing.T, boxParameter string) *http.Request {
	v := url.Values{}

	if boxParameter != "" {
		v.Set("box", boxParameter)
	}

	urlString := "https://127.0.0.1:8080?" + v.Encode()

	req, err := http.NewRequest(http.MethodGet, urlString, nil)
	require.NoError(t, err)

	return req
}

func makePostRequest(t *testing.T, boxValue string) *http.Request {
	urlString := "https://127.0.0.1:8080"

	body, err := json.Marshal(map[string]string{
		"box": boxValue,
	})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, urlString, bytes.NewBuffer(body))
	require.NoError(t, err)

	return req
}

func mustMarshal(t *testing.T, v interface{}) []byte {
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}
