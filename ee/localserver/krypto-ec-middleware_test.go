package localserver

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/krypto/pkg/challenge"
	"github.com/kolide/krypto/pkg/echelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKryptoEcMiddleware(t *testing.T) {
	t.Parallel()

	myKey, err := echelper.GenerateEcdsaKey()
	require.NoError(t, err)

	counterpartyKey, err := echelper.GenerateEcdsaKey()
	require.NoError(t, err)

	challengeId := []byte(ulid.New())
	challengeData := []byte(ulid.New())
	expectedCmd := "id"
	cmdReq := mustMarshal(t, cmdRequestType{Cmd: expectedCmd})

	var tests = []struct {
		name      string
		challenge func() ([]byte, *[32]byte)
		loggedErr string
	}{
		{
			name:      "no command",
			challenge: func() ([]byte, *[32]byte) { return []byte(""), nil },
			loggedErr: "no data in box query parameter",
		},
		{
			name:      "no signature",
			challenge: func() ([]byte, *[32]byte) { return []byte("aGVsbG8gd29ybGQK"), nil },
			loggedErr: "unable to unmarshal box",
		},
		{
			name: "malformed cmd",
			challenge: func() ([]byte, *[32]byte) {
				challenge, _, err := challenge.Generate(counterpartyKey, challengeId, challengeData, []byte("malformed stuff"))
				require.NoError(t, err)
				return challenge, nil
			},
			loggedErr: "unable to unmarshal cmd request",
		},
		{
			name: "wrong signature",
			challenge: func() ([]byte, *[32]byte) {
				malloryKey, err := echelper.GenerateEcdsaKey()
				require.NoError(t, err)
				challenge, _, err := challenge.Generate(malloryKey, challengeId, challengeData, cmdReq)
				return challenge, nil
			},
			loggedErr: "unable to verify signature",
		},
		{
			name: "works",
			challenge: func() ([]byte, *[32]byte) {
				challenge, priv, err := challenge.Generate(counterpartyKey, challengeId, challengeData, cmdReq)
				require.NoError(t, err)
				return challenge, priv
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var logBytes bytes.Buffer

			// set up middlewares
			kryptoEcMiddleware := newKryptoEcMiddleware(log.NewLogfmtLogger(&logBytes), myKey, counterpartyKey.PublicKey)
			require.NoError(t, err)

			challengeBytes, privateEncryptionKey := tt.challenge()

			// generate the response we want the handler to return
			responseBytes := []byte{}

			if tt.loggedErr == "" {
				outerChallenge, err := challenge.UnmarshalChallenge(challengeBytes)
				require.NoError(t, err)
				require.NoError(t, outerChallenge.Verify(counterpartyKey.PublicKey))

				responseData := []byte(ulid.New())

				responseBytes, err = outerChallenge.Respond(myKey, responseData)
				require.NoError(t, err)
			}

			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(base64.StdEncoding.EncodeToString(responseBytes)))
			})

			// give our test handler to the determiner
			h := NewKryptoDeterminerMiddleware(log.NewLogfmtLogger(&logBytes), nil, kryptoEcMiddleware.Wrap(testHandler))

			// make the request
			req := makeRequest(t, base64.StdEncoding.EncodeToString(challengeBytes))
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			if tt.loggedErr != "" {
				assert.Equal(t, http.StatusUnauthorized, rr.Code)
				assert.Contains(t, logBytes.String(), tt.loggedErr)
				return
			}

			require.Equal(t, http.StatusOK, rr.Code)
			require.NotEmpty(t, rr.Body.String())

			// try to open the response
			returnedResponseBytes, err := base64.StdEncoding.DecodeString(rr.Body.String())
			require.NoError(t, err)

			responseUnmarshalled, err := challenge.UnmarshalResponse(returnedResponseBytes)
			_, err = responseUnmarshalled.Open(*privateEncryptionKey)
			require.NoError(t, err)
		})
	}
}
