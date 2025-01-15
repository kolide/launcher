package localserver

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/krypto/pkg/challenge"
	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/ee/localserver/mocks"

	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestKryptoEcMiddleware(t *testing.T) {
	t.Parallel()

	koldieSessionId := ulid.New()

	var tests = []struct {
		name                                     string
		cmdReqHeaders                            map[string][]string
		cmdReqCallbackHeaders                    map[string][]string
		expectedResponseHeaders                  map[string][]string
		expectedCallbackHeaders                  map[string][]string
		expectedPresenceDetectionCallbackHeaders map[string][]string
	}{
		{
			name: "with presence detection call back",
			cmdReqHeaders: map[string][]string{
				kolidePresenceDetectionIntervalHeaderKey: {"0s"},
			},
			cmdReqCallbackHeaders: map[string][]string{
				kolideSessionIdHeaderKey: {koldieSessionId},
			},
			expectedResponseHeaders: map[string][]string{
				kolideOsHeaderKey:        {runtime.GOOS},
				kolideArchHeaderKey:      {runtime.GOARCH},
				kolideSessionIdHeaderKey: {koldieSessionId},
			},
			expectedCallbackHeaders: map[string][]string{
				kolideArchHeaderKey:      {runtime.GOARCH},
				kolideOsHeaderKey:        {runtime.GOOS},
				kolideSessionIdHeaderKey: {koldieSessionId},
			},
			expectedPresenceDetectionCallbackHeaders: map[string][]string{
				kolideArchHeaderKey:                               {runtime.GOARCH},
				kolideOsHeaderKey:                                 {runtime.GOOS},
				kolideSessionIdHeaderKey:                          {koldieSessionId},
				kolideDurationSinceLastPresenceDetectionHeaderKey: {"0s"},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// set up keys
			remoteServerPrivateKey := mustGenEcdsaKey(t)
			localServerPrivateKey := mustGenEcdsaKey(t)

			// create some challenge info
			challengeId := []byte(ulid.New())
			challengeData := []byte(ulid.New())

			callbackWaitGroup := sync.WaitGroup{}
			callbackWaitGroup.Add(2)

			// assume that if we have presence detection headers, we should have a presence detection callback
			if tt.expectedPresenceDetectionCallbackHeaders != nil && runtime.GOOS != "linux" {
				callbackWaitGroup.Add(2)
			}

			// this is the key we can use to open the response
			// it gets set later
			var challengePrivateKey *[32]byte

			callbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer callbackWaitGroup.Done()

				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)

				opened := mustOpenKryptoResponse(t, mustExtractJsonProperty[string](t, body, "Response"), challengePrivateKey)

				headers := mustExtractJsonProperty[map[string][]string](t, opened.ResponseData, "headers")

				// assume that if we have presence detection header, this is the presence detection callback
				if _, ok := headers[kolideDurationSinceLastPresenceDetectionHeaderKey]; ok {
					require.Equal(t, tt.expectedPresenceDetectionCallbackHeaders, headers,
						"presence detection callback headers should match expected",
					)
					return
				}

				require.Equal(t, tt.expectedCallbackHeaders, headers,
					"callback headers should match expected",
				)
			}))

			cmdReq := v2CmdRequestType{
				Path:            "whatevs",
				Body:            []byte(randomStringWithSqlCharacters(t, 100000)),
				Headers:         tt.cmdReqHeaders,
				CallbackUrl:     callbackServer.URL,
				CallbackHeaders: tt.cmdReqCallbackHeaders,
			}

			challengeKryptoBoxB64, challengePrivateKey := mustGenerateChallenge(t, remoteServerPrivateKey, challengeId, challengeData, mustMarshal(t, cmdReq))

			responseBody := mustMarshal(t, map[string]any{
				"body": ulid.New(),
			})

			for _, req := range []*http.Request{mustMakeGetRequest(t, challengeKryptoBoxB64), mustMakePostRequest(t, challengeKryptoBoxB64)} {
				req := req
				t.Run(req.Method, func(t *testing.T) {
					// t.Parallel()

					var logBytes bytes.Buffer
					slogger := multislogger.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
						Level: slog.LevelDebug,
					})).Logger

					mockPresenceDetector := mocks.NewPresenceDetector(t)
					mockPresenceDetector.On("DetectPresence", mock.AnythingOfType("string"), mock.AnythingOfType("Duration")).Return(0*time.Second, nil).Maybe()

					// set up middlewares
					kryptoEcMiddleware := newKryptoEcMiddleware(slogger, localServerPrivateKey, remoteServerPrivateKey.PublicKey, mockPresenceDetector)

					rr := httptest.NewRecorder()
					// give our middleware with the test handler to the determiner
					kryptoEcMiddleware.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						// make sure all the request headers are present
						for k, v := range cmdReq.Headers {
							require.Equal(t, v[0], r.Header.Get(k))
						}

						w.Write(responseBody)
					})).ServeHTTP(rr, req)

					require.Contains(t, logBytes.String(), multislogger.KolideSessionIdKey.String())
					require.Contains(t, logBytes.String(), koldieSessionId)
					require.Contains(t, logBytes.String(), multislogger.SpanIdKey.String())

					require.Equal(t, http.StatusOK, rr.Code)

					require.Equal(t, kolideKryptoEccHeader20230130Value, rr.Header().Get(kolideKryptoHeaderKey))

					opened := mustOpenKryptoResponse(t, rr.Body.String(), challengePrivateKey)

					require.Equal(t, mustExtractJsonProperty[string](t, responseBody, "body"), mustExtractJsonProperty[string](t, opened.ResponseData, "body"),
						"returned response body should match the expected response body",
					)

					require.WithinDuration(t, time.Now(), time.Unix(opened.Timestamp, 0), time.Second*5)

					responseHeaders := mustExtractJsonProperty[map[string][]string](t, opened.ResponseData, "headers")
					require.Equal(t, tt.expectedResponseHeaders, responseHeaders)
				})
			}

			// wait for the callbacks to finish so callback server can run tests
			callbackWaitGroup.Wait()
		})
	}
}

func TestKryptoEcMiddlewareErrors(t *testing.T) {
	t.Parallel()

	// set up keys
	remoteServerPrivateKey := mustGenEcdsaKey(t)
	localServerPrivateKey := mustGenEcdsaKey(t)

	var tests = []struct {
		name      string
		loggedErr string
		challenge func() string
	}{
		{
			name:      "no command",
			loggedErr: "failed to extract box from request",
			challenge: func() string {
				return ""
			},
		},
		{
			name: "malformed cmd",
			challenge: func() string {
				challenge, _ := mustGenerateChallenge(t, remoteServerPrivateKey, []byte(ulid.New()), []byte(ulid.New()), []byte("malformed stuff"))
				return challenge
			},
			loggedErr: "unable to unmarshal cmd request",
		},
		{
			name: "wrong signature",
			challenge: func() string {
				malloryKey := mustGenEcdsaKey(t)
				challenge, _ := mustGenerateChallenge(t, malloryKey, []byte(ulid.New()), []byte(ulid.New()), mustMarshal(t, v2CmdRequestType{}))
				return challenge
			},
			loggedErr: "unable to verify signature",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			getRequest := mustMakeGetRequest(t, tt.challenge())

			for _, req := range []*http.Request{getRequest /* makePostRequest(t, encodedChallenge) */} {
				req := req
				t.Run(req.Method, func(t *testing.T) {
					// t.Parallel()

					var logBytes bytes.Buffer
					slogger := multislogger.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
						Level: slog.LevelDebug,
					})).Logger

					mockPresenceDetector := mocks.NewPresenceDetector(t)
					mockPresenceDetector.On("DetectPresence", mock.AnythingOfType("string"), mock.AnythingOfType("Duration")).Return(0*time.Second, nil).Maybe()

					// set up middlewares
					kryptoEcMiddleware := newKryptoEcMiddleware(slogger, localServerPrivateKey, remoteServerPrivateKey.PublicKey, mockPresenceDetector)

					rr := httptest.NewRecorder()

					// give our middleware with the test handler to the determiner
					kryptoEcMiddleware.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Write([]byte("what evs"))
					})).ServeHTTP(rr, req)

					require.Contains(t, logBytes.String(), tt.loggedErr)

					require.NotEqual(t, http.StatusOK, rr.Code,
						"should have no 200 code on failure",
					)
				})
			}
		})
	}
}

func Test_AllowedOrigin(t *testing.T) {
	t.Parallel()

	counterpartyKey := mustGenEcdsaKey(t)

	var tests = []struct {
		name            string
		requestOrigin   string
		requestReferrer string
		allowedOrigins  []string
		logStr          string
		expectedStatus  int
	}{
		{
			name:           "no allowed specified",
			requestOrigin:  "https://auth.example.com",
			expectedStatus: http.StatusOK,
			logStr:         "origin is allowed by default",
		},
		{
			name:           "no allowed specified missing origin",
			expectedStatus: http.StatusOK,
			logStr:         "origin is allowed by default",
		},
		{
			name:           "allowed specified missing origin",
			allowedOrigins: []string{"https://auth.example.com", "https://login.example.com"},
			expectedStatus: http.StatusUnauthorized,
			logStr:         "origin is not allowed",
		},
		{
			name:           "allowed specified origin mismatch",
			allowedOrigins: []string{"https://auth.example.com", "https://login.example.com"},
			requestOrigin:  "https://not-it.example.com",
			expectedStatus: http.StatusUnauthorized,
			logStr:         "origin is not allowed",
		},
		{
			name:           "scheme mismatch",
			allowedOrigins: []string{"https://auth.example.com"},
			requestOrigin:  "http://auth.example.com",
			expectedStatus: http.StatusUnauthorized,
			logStr:         "origin is not allowed",
		},
		{
			name:           "allowed specified origin matches",
			allowedOrigins: []string{"https://auth.example.com", "https://login.example.com"},
			requestOrigin:  "https://auth.example.com",
			expectedStatus: http.StatusOK,
			logStr:         "origin matches allowlist",
		},
		{
			name:           "allowed specified origin matches 2",
			allowedOrigins: []string{"https://auth.example.com", "https://login.example.com"},
			requestOrigin:  "https://login.example.com",
			expectedStatus: http.StatusOK,
			logStr:         "origin matches allowlist",
		},
		{
			name:           "allowed specified origin matches casing",
			allowedOrigins: []string{"https://auth.example.com", "https://login.example.com"},
			requestOrigin:  "https://AuTh.ExAmPlE.cOm",
			expectedStatus: http.StatusOK,
			logStr:         "origin matches allowlist",
		},
		{
			name:            "no allowed specified origin, but acceptable referer is present",
			allowedOrigins:  []string{"https://auth.example.com", "https://login.example.com"},
			requestReferrer: "https://auth.example.com",
			expectedStatus:  http.StatusOK,
			logStr:          "origin matches allowlist",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmdReqBody := []byte(randomStringWithSqlCharacters(t, 100000))

			cmdReq := v2CmdRequestType{
				Path:           "whatevs",
				Body:           cmdReqBody,
				AllowedOrigins: tt.allowedOrigins,
			}

			challengeKryptoBoxB64, privateEncryptionKey := mustGenerateChallenge(t, counterpartyKey, []byte(ulid.New()), []byte(ulid.New()), mustMarshal(t, cmdReq))

			responseData := []byte(ulid.New())

			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reqBodyRaw, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				defer r.Body.Close()

				require.Equal(t, cmdReqBody, reqBodyRaw)
				w.Write(responseData)
			})

			var logBytes bytes.Buffer
			slogger := multislogger.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			})).Logger

			mockPresenceDetector := mocks.NewPresenceDetector(t)
			mockPresenceDetector.On("DetectPresence", mock.AnythingOfType("string"), mock.AnythingOfType("Duration")).Return(0*time.Second, nil).Maybe()

			// set up middlewares
			kryptoEcMiddleware := newKryptoEcMiddleware(slogger, mustGenEcdsaKey(t), counterpartyKey.PublicKey, mockPresenceDetector)

			h := kryptoEcMiddleware.Wrap(testHandler)

			req := mustMakeGetRequest(t, challengeKryptoBoxB64)
			req.Header.Set("origin", tt.requestOrigin)
			req.Header.Set("referer", tt.requestReferrer)

			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			// FIXME: add some log string tests
			//spew.Dump(logBytes.String(), tt.logStr)

			require.Equal(t, tt.expectedStatus, rr.Code)

			if tt.logStr != "" {
				assert.Contains(t, logBytes.String(), tt.logStr)
			}

			if tt.expectedStatus != http.StatusOK {
				return
			}

			mustOpenKryptoResponse(t, rr.Body.String(), privateEncryptionKey)
		})
	}

}

func mustGenEcdsaKey(t *testing.T) *ecdsa.PrivateKey {
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

func mustGenerateChallenge(t *testing.T, key *ecdsa.PrivateKey, challengeId, challengeData []byte, cmdReq []byte) (string, *[32]byte) {
	challenge, priv, err := challenge.Generate(key, challengeId, challengeData, cmdReq)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(challenge), priv
}

func mustOpenKryptoResponse(t *testing.T, responseB64 string, privateEncryptionKey *[32]byte) *challenge.InnerResponse {
	returnedResponseBytes, err := base64.StdEncoding.DecodeString(responseB64)
	require.NoError(t, err)

	responseUnmarshalled, err := challenge.UnmarshalResponse(returnedResponseBytes)
	require.NoError(t, err)
	// require.Equal(t, challengeId, responseUnmarshalled.ChallengeId)

	opened, err := responseUnmarshalled.Open(privateEncryptionKey)
	require.NoError(t, err)
	return opened
	// require.Equal(t, challengeData, opened.ChallengeData)
}

func mustMakeGetRequest(t *testing.T, challengeKryptoBoxB64 string) *http.Request {
	v := url.Values{}
	v.Set("box", challengeKryptoBoxB64)

	urlString := fmt.Sprint("https://127.0.0.1:8080?", v.Encode())

	req, err := http.NewRequest(http.MethodGet, urlString, nil)
	require.NoError(t, err)

	return req
}

func mustMakePostRequest(t *testing.T, challengeKryptoBoxB64 string) *http.Request {
	urlString := "https://127.0.0.1:8080"

	body, err := json.Marshal(map[string]string{
		"box": challengeKryptoBoxB64,
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

func mustExtractJsonProperty[T any](t *testing.T, jsonData []byte, property string) T {
	var result map[string]json.RawMessage

	// Unmarshal the JSON data into a map with json.RawMessage
	require.NoError(t, json.Unmarshal(jsonData, &result))

	// Retrieve the field from the map
	value, ok := result[property]
	require.True(t, ok, "property %s not found", property)

	// Unmarshal the value into the type T
	var extractedValue T
	require.NoError(t, json.Unmarshal(value, &extractedValue))

	return extractedValue
}
