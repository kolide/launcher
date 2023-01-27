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
)

func TestKryptoEcMiddlewareUnwrap(t *testing.T) {
	t.Parallel()

	myKey, err := echelper.GenerateEcdsaKey()
	require.NoError(t, err)

	counterpartyKey, err := echelper.GenerateEcdsaKey()
	require.NoError(t, err)

	challengeId := []byte(ulid.New())
	challengeData := []byte(ulid.New())
	expectedCmd := ulid.New()
	cmdReq := mustMarshal(t, cmdRequestType{Cmd: expectedCmd})

	var tests = []struct {
		name      string
		boxParam  func() []byte
		loggedErr string
	}{
		{
			name:      "no command",
			boxParam:  func() []byte { return []byte("") },
			loggedErr: "no data in box query parameter",
		},
		{
			name:      "no signature",
			boxParam:  func() []byte { return []byte("aGVsbG8gd29ybGQK") },
			loggedErr: "unable to verify box",
		},
		{
			name: "malformed cmd",
			boxParam: func() []byte {
				challenge, _, err := challenge.Generate(counterpartyKey, challengeId, challengeData, []byte("malformed stuff"))
				require.NoError(t, err)
				return challenge
			},
			loggedErr: "unable to unmarshal cmd request",
		},
		{
			name: "wrong signature",
			boxParam: func() []byte {
				malloryKey, err := echelper.GenerateEcdsaKey()
				require.NoError(t, err)
				challenge, _, err := challenge.Generate(malloryKey, challengeId, challengeData, cmdReq)
				return challenge
			},
			loggedErr: "unable to verify signature",
		},
		{
			name: "works",
			boxParam: func() []byte {
				challenge, _, err := challenge.Generate(counterpartyKey, challengeId, challengeData, cmdReq)
				require.NoError(t, err)
				return challenge
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

			boxParam := base64.StdEncoding.EncodeToString(tt.boxParam())
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
	expectedCmd := ulid.New()
	cmdReq := mustMarshal(t, cmdRequestType{Cmd: expectedCmd})

	var tests = []struct {
		name          string
		challengeFunc func() ([]byte, *[32]byte)
		loggedErr     string
	}{
		{
			name:          "no command",
			challengeFunc: func() ([]byte, *[32]byte) { return []byte(""), nil },
			loggedErr:     "no data in box query parameter",
		},
		{
			name:          "no signature",
			challengeFunc: func() ([]byte, *[32]byte) { return []byte("aGVsbG8gd29ybGQK"), nil },
			loggedErr:     "failed to wrap response",
		},
		{
			name: "works",
			challengeFunc: func() ([]byte, *[32]byte) {
				challenge, privKey, err := challenge.Generate(counterpartyKey, challengeId, challengeData, cmdReq)
				require.NoError(t, err)
				return challenge, privKey
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
			req := makeEcWrapRequest(t, base64.StdEncoding.EncodeToString(generatedChallenge))

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

			challengeBox, err := challenge.UnmarshalResponse(resultBytes)
			require.NoError(t, err)

			opened, err := challengeBox.Open(*privKey)
			require.NoError(t, err)
			assert.Equal(t, challengeData, opened.ChallengeData)

			// now test png unwrap
			h = kryptoDeterminerMiddleware.determineKryptoWrapPng(makeTestHandler(t))
			req = makeEcWrapRequest(t, base64.StdEncoding.EncodeToString(generatedChallenge))

			rr = httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			assert.Equal(t, http.StatusOK, rr.Code)
			assert.NotEmpty(t, rr.Body.String())
			b64string = rr.Body.String()
			resultBytes, err = base64.StdEncoding.DecodeString(b64string)
			require.NoError(t, err)

			challengeBox, err = challenge.UnmarshalResponsePng(resultBytes)
			opened, err = challengeBox.Open(*privKey)
			require.NoError(t, err)
			assert.Equal(t, challengeData, opened.ChallengeData)
		})
	}
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
