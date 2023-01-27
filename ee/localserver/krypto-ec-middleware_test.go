package localserver

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/krypto/pkg/challenge"
	"github.com/kolide/krypto/pkg/echelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

func TestKryptoEcMiddlewareUnwrap(t *testing.T) {
	t.Parallel()

	myKey, err := echelper.GenerateEcdsaKey()
	require.NoError(t, err)

	counterpartyKey, err := echelper.GenerateEcdsaKey()
	require.NoError(t, err)

	expectedCmd := ulid.New()
	challengeId := []byte(ulid.New())
	cmdReq := mustMarshal(t, cmdRequestType{Cmd: expectedCmd})

	var tests = []struct {
		name      string
		boxParam  func() string
		loggedErr string
	}{
		{
			name:      "no command",
			boxParam:  func() string { return "" },
			loggedErr: "no data in box query parameter",
		},
		{
			name:      "bad base64",
			boxParam:  func() string { return "This is not base64" },
			loggedErr: "unable to base64 decode box",
		},
		{
			name:      "no signature",
			boxParam:  func() string { return "aGVsbG8gd29ybGQK" },
			loggedErr: "unable to verify box",
		},
		{
			name: "malformed cmd",
			boxParam: func() string {
				challenge, _, err := challenge.Generate(counterpartyKey, challengeId, []byte("malformed stuff"))
				require.NoError(t, err)
				return mustMarshallChallenge(t, challenge)
			},
			loggedErr: "unable to unmarshal cmd request",
		},
		{
			name: "wrong signature",
			boxParam: func() string {
				malloryKey, err := echelper.GenerateEcdsaKey()
				require.NoError(t, err)
				challenge, _, err := challenge.Generate(malloryKey, challengeId, cmdReq)
				return mustMarshallChallenge(t, challenge)
			},
			loggedErr: "unable to verify signature",
		},
		{
			name: "works",
			boxParam: func() string {
				challenge, _, err := challenge.Generate(counterpartyKey, challengeId, cmdReq)
				require.NoError(t, err)
				return mustMarshallChallenge(t, challenge)
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var logBytes bytes.Buffer

			kryptoEcMiddleware := newKryptoEcMiddleware(log.NewLogfmtLogger(&logBytes), myKey, counterpartyKey.PublicKey)
			require.NoError(t, err)

			kryptoDeterminerMiddleware := NewKryptoDeterminerMiddleware(log.NewLogfmtLogger(&logBytes), nil, kryptoEcMiddleware)

			boxParam := tt.boxParam()
			h := kryptoDeterminerMiddleware.determineKryptoUnwrap(makeTestHandler(t))
			req := makeRequest(t, boxParam)

			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			if tt.loggedErr != "" {
				assert.Equal(t, http.StatusUnauthorized, rr.Code)
				assert.Contains(t, logBytes.String(), tt.loggedErr)
				return
			}

			assert.Equal(t, http.StatusOK, rr.Code)
			assert.NotEmpty(t, rr.Body.String())
			assert.Equal(t, fmt.Sprintf("https://127.0.0.1:8080/%s?box=%s&krypto-type=ec", expectedCmd, url.QueryEscape(boxParam)), rr.Body.String())
		})
	}
}

func TestKryptoEcMiddlewareWrap(t *testing.T) {
	t.Parallel()

	myKey, err := echelper.GenerateEcdsaKey()
	require.NoError(t, err)

	counterpartyKey, err := echelper.GenerateEcdsaKey()
	require.NoError(t, err)

	challengeData := []byte(ulid.New())
	challengeId := []byte(ulid.New())

	var tests = []struct {
		name          string
		challengeFunc func() (string, *[32]byte)
		loggedErr     string
	}{
		{
			name:          "no command",
			challengeFunc: func() (string, *[32]byte) { return "", nil },
			loggedErr:     "no data in box query parameter",
		},
		{
			name:          "bad base64",
			challengeFunc: func() (string, *[32]byte) { return "This is not base64", nil },
			loggedErr:     "unable to base64 decode box",
		},
		{
			name:          "no signature",
			challengeFunc: func() (string, *[32]byte) { return "aGVsbG8gd29ybGQK", nil },
			loggedErr:     "unable to marshall outer challenge",
		},
		{
			name: "wrong signature",
			challengeFunc: func() (string, *[32]byte) {
				malloryKey, err := echelper.GenerateEcdsaKey()
				require.NoError(t, err)
				challenge, priv, err := challenge.Generate(malloryKey, challengeId, challengeData)
				return mustMarshallChallenge(t, challenge), priv
			},
			loggedErr: "invalid signature",
		},
		{
			name: "works",
			challengeFunc: func() (string, *[32]byte) {
				challenge, privKey, err := challenge.Generate(counterpartyKey, challengeId, challengeData)
				require.NoError(t, err)
				return mustMarshallChallenge(t, challenge), privKey
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var logBytes bytes.Buffer

			kryptoEcMiddleware := newKryptoEcMiddleware(log.NewLogfmtLogger(&logBytes), myKey, counterpartyKey.PublicKey)
			require.NoError(t, err)

			kryptoDeterminerMiddleware := NewKryptoDeterminerMiddleware(log.NewLogfmtLogger(&logBytes), nil, kryptoEcMiddleware)

			generatedChallenge, privKey := tt.challengeFunc()

			h := kryptoDeterminerMiddleware.determineKryptoWrap(makeTestHandler(t))
			req := makeEcWrapRequest(t, generatedChallenge)

			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			if tt.loggedErr != "" {
				assert.Equal(t, http.StatusUnauthorized, rr.Code)
				assert.Contains(t, logBytes.String(), tt.loggedErr)
				return
			}

			assert.Equal(t, http.StatusOK, rr.Code)
			assert.NotEmpty(t, rr.Body.String())
			b64string := rr.Body.String()
			resultBytes, err := base64.StdEncoding.DecodeString(b64string)
			require.NoError(t, err)

			var challengeResponse challenge.OuterResponse
			require.NoError(t, msgpack.Unmarshal(resultBytes, &challengeResponse))
			assert.Equal(t, challengeId, challengeResponse.ChallengeId)

			opened, err := challenge.OpenResponse(*privKey, challengeResponse)
			require.NoError(t, err)
			assert.Equal(t, challengeData, opened.ChallengeData)

			h = kryptoDeterminerMiddleware.determineKryptoWrapPng(makeTestHandler(t))
			req = makeEcWrapRequest(t, generatedChallenge)

			rr = httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			assert.Equal(t, http.StatusOK, rr.Code)
			assert.NotEmpty(t, rr.Body.String())
			b64string = rr.Body.String()
			resultBytes, err = base64.StdEncoding.DecodeString(b64string)
			require.NoError(t, err)

			opened, err = challenge.OpenResponsePng(*privKey, resultBytes)
			require.NoError(t, err)
			assert.Equal(t, challengeData, opened.ChallengeData)
		})
	}
}

func mustMarshallChallenge(t *testing.T, challenge *challenge.OuterChallenge) string {
	challengeBytes, err := msgpack.Marshal(challenge)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(challengeBytes)
}

func makeEcWrapRequest(t *testing.T, boxParameter string) *http.Request {
	v := url.Values{}

	if boxParameter != "" {
		v.Set("box", boxParameter)
	}

	v.Set("krypto-type", "ec")

	urlString := "https://127.0.0.1:8080?" + v.Encode()

	req, err := http.NewRequest("GET", urlString, nil)
	require.NoError(t, err)

	return req
}
