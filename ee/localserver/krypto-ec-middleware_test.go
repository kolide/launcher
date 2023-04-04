package localserver

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/krypto/pkg/challenge"
	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/pkg/agent/keys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKryptoEcMiddleware(t *testing.T) {
	t.Parallel()

	counterpartyKey, err := echelper.GenerateEcdsaKey()
	require.NoError(t, err)

	challengeId := []byte(ulid.New())
	challengeData := []byte(ulid.New())

	cmdReqBody := []byte(randomStringWithSqlCharacters(t, 100000))

	cmdReq := mustMarshal(t, v2CmdRequestType{
		Path: "whatevs",
		Body: cmdReqBody,
	})

	var tests = []struct {
		name                    string
		localDbKey, hardwareKey crypto.Signer
		challenge               func() ([]byte, *[32]byte)
		loggedErr               string
		handler                 http.HandlerFunc
		responseData            []byte
	}{
		{
			name:       "no command",
			localDbKey: ecdsaKey(t),
			challenge:  func() ([]byte, *[32]byte) { return []byte(""), nil },
			loggedErr:  "failed to extract box from request",
		},
		{
			name:       "malformed cmd",
			localDbKey: ecdsaKey(t),
			challenge: func() ([]byte, *[32]byte) {
				challenge, _, err := challenge.Generate(counterpartyKey, challengeId, challengeData, []byte("malformed stuff"))
				require.NoError(t, err)
				return challenge, nil
			},
			loggedErr: "unable to unmarshal cmd request",
		},
		{
			name:       "wrong signature",
			localDbKey: ecdsaKey(t),
			challenge: func() ([]byte, *[32]byte) {
				malloryKey, err := echelper.GenerateEcdsaKey()
				require.NoError(t, err)
				challenge, _, err := challenge.Generate(malloryKey, challengeId, challengeData, cmdReq)
				require.NoError(t, err)
				return challenge, nil
			},
			loggedErr: "unable to verify signature",
		},
		{
			name:       "not found 404",
			localDbKey: ecdsaKey(t),
			challenge: func() ([]byte, *[32]byte) {
				challenge, priv, err := challenge.Generate(counterpartyKey, challengeId, challengeData, cmdReq)
				require.NoError(t, err)
				return challenge, priv
			},
			handler:      http.NotFound,
			responseData: []byte("404 page not found\n"),
		},
		{
			name:        "works with hardware key",
			localDbKey:  ecdsaKey(t),
			hardwareKey: ecdsaKey(t),
			challenge: func() ([]byte, *[32]byte) {
				challenge, priv, err := challenge.Generate(counterpartyKey, challengeId, challengeData, cmdReq)
				require.NoError(t, err)
				return challenge, priv
			},
		},
		{
			name:       "works with nil hardware key",
			localDbKey: ecdsaKey(t),
			challenge: func() ([]byte, *[32]byte) {
				challenge, priv, err := challenge.Generate(counterpartyKey, challengeId, challengeData, cmdReq)
				require.NoError(t, err)
				return challenge, priv
			},
		},
		{
			name:        "works with noop hardware key",
			localDbKey:  ecdsaKey(t),
			hardwareKey: keys.Noop,
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

			responseData := tt.responseData
			// generate the response we want the handler to return
			if responseData == nil {
				responseData = []byte(ulid.New())
			}

			testHandler := tt.handler

			// this handler is what will respond to the request made by the kryptoEcMiddleware.Wrap handler
			// in this test we just want it to regurgitate the response data we defined above
			// this should match the responseData in the opened response
			if testHandler == nil {
				testHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					reqBodyRaw, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					defer r.Body.Close()

					require.Equal(t, cmdReqBody, reqBodyRaw)
					w.Write(responseData)
				})
			}

			challengeBytes, privateEncryptionKey := tt.challenge()

			encodedChallenge := base64.StdEncoding.EncodeToString(challengeBytes)
			for _, req := range []*http.Request{makeGetRequest(t, encodedChallenge), makePostRequest(t, encodedChallenge)} {
				req := req
				t.Run(req.Method, func(t *testing.T) {
					t.Parallel()

					var logBytes bytes.Buffer

					// set up middlewares
					kryptoEcMiddleware := newKryptoEcMiddleware(log.NewLogfmtLogger(&logBytes), tt.localDbKey, tt.hardwareKey, counterpartyKey.PublicKey)
					require.NoError(t, err)

					// give our middleware with the test handler to the determiner
					h := kryptoEcMiddleware.Wrap(testHandler)

					rr := httptest.NewRecorder()
					h.ServeHTTP(rr, req)

					if tt.loggedErr != "" {
						assert.Equal(t, http.StatusUnauthorized, rr.Code)
						assert.Contains(t, logBytes.String(), tt.loggedErr)
						return
					}

					require.Equal(t, http.StatusOK, rr.Code)
					require.NotEmpty(t, rr.Body.String())

					require.Equal(t, kolideKryptoEccHeader20230130Value, rr.Header().Get(kolideKryptoHeaderKey))

					// try to open the response
					returnedResponseBytes, err := base64.StdEncoding.DecodeString(rr.Body.String())
					require.NoError(t, err)

					responseUnmarshalled, err := challenge.UnmarshalResponse(returnedResponseBytes)
					require.NoError(t, err)
					require.Equal(t, challengeId, responseUnmarshalled.ChallengeId)

					opened, err := responseUnmarshalled.Open(*privateEncryptionKey)
					require.NoError(t, err)
					require.Equal(t, challengeData, opened.ChallengeData)
					require.Equal(t, responseData, opened.ResponseData)
					require.WithinDuration(t, time.Now(), time.Unix(opened.Timestamp, 0), time.Second*5)
				})
			}
		})
	}
}

func ecdsaKey(t *testing.T) *ecdsa.PrivateKey {
	key, err := echelper.GenerateEcdsaKey()
	require.NoError(t, err)
	return key
}

// tried to add all the characters that may appear in sql
const randomStringCharsetForSqlEncoding = "aA0_'%!@#&()-[{}]:;',?/*`~$^+=<>\""

func randomStringWithSqlCharacters(t *testing.T, n int) string {
	maxInt := big.NewInt(int64(len(randomStringCharsetForSqlEncoding)))

	sb := strings.Builder{}
	sb.Grow(n)
	for i := 0; i < n; i++ {
		char, err := rand.Int(rand.Reader, maxInt)
		require.NoError(t, err)

		sb.WriteByte(randomStringCharsetForSqlEncoding[int(char.Int64())])
	}
	return sb.String()
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
