package localserver

import (
	"bytes"
	"crypto/rsa"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

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

	var tests = []struct {
		name      string
		req       *http.Request
		loggedErr string
	}{
		{
			name:      "no command",
			req:       makeUnSignedRequest(t, bytes.NewBufferString("/id")),
			loggedErr: "No command",
		},
		{
			name:      "no signature",
			req:       addCmdHeaderToRequest(makeUnSignedRequest(t, bytes.NewBufferString("/id")), "/id"),
			loggedErr: "No signature",
		},
		{
			name:      "mismatched signature",
			req:       addCmdHeaderToRequest(signRequest(t, addCmdHeaderToRequest(makeUnSignedRequest(t, bytes.NewBufferString("/id")), "/id"), counterpartyKey), "/different"),
			loggedErr: "signature mismatch",
		},
		{
			name: "signed",
			req:  signRequest(t, addCmdHeaderToRequest(makeUnSignedRequest(t, bytes.NewBufferString("/id")), "/id"), counterpartyKey),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var logBytes bytes.Buffer

			ls := &localServer{
				logger:    log.NewLogfmtLogger(&logBytes),
				myKey:     myKey,
				serverKey: counterpartyPub,
			}

			answer := ulid.New()
			h := ls.UnwrapV0Handler(makeTestHandler(t, answer))

			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, tt.req)

			if tt.loggedErr != "" {
				assert.Equal(t, http.StatusUnauthorized, rr.Code)
				assert.Contains(t, logBytes.String(), tt.loggedErr)
				return
			}

			assert.Equal(t, http.StatusOK, rr.Code)
			assert.Equal(t, answer, rr.Body.String())
		})
	}

}

func TestMakeTestHander(t *testing.T) {
	t.Parallel()

	answer := ulid.New()
	h := makeTestHandler(t, answer)

	req, err := http.NewRequest("GET", "http://localhost:8080", nil)
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, answer, rr.Body.String())
}

func makeTestHandler(t *testing.T, answer string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(answer))
	})
}

func makeUnSignedRequest(t *testing.T, body *bytes.Buffer) *http.Request {
	req, err := http.NewRequest("GET", "http://localhost:8080", body)
	require.NoError(t, err)
	return req
}

func addCmdHeaderToRequest(req *http.Request, cmd string) *http.Request {
	req.Header.Set(v0CmdHeader, cmd)
	return req
}

func signRequest(t *testing.T, req *http.Request, counterparty *rsa.PrivateKey) *http.Request {
	sig, err := krypto.RsaSign(counterparty, []byte(req.Header.Get(v0CmdHeader)))
	require.NoError(t, err)

	req.Header.Set(v0CmdSignature, base64.StdEncoding.EncodeToString(sig))
	return req
}
