package localserver

import (
	"bytes"
	"context"
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
	"sync/atomic"
	"testing"
	"time"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/krypto/pkg/challenge"
	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/ee/localserver/mocks"

	"github.com/kolide/launcher/ee/agent/types"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
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
				kolideMunemoHeaderKey:                    {"test-munemo"},
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
		{
			name: "without session id",
			cmdReqHeaders: map[string][]string{
				kolidePresenceDetectionIntervalHeaderKey: {"0s"},
			},
			cmdReqCallbackHeaders: map[string][]string{},
			expectedResponseHeaders: map[string][]string{
				kolideOsHeaderKey:   {runtime.GOOS},
				kolideArchHeaderKey: {runtime.GOARCH},
			},
			expectedCallbackHeaders: map[string][]string{
				kolideArchHeaderKey: {runtime.GOARCH},
				kolideOsHeaderKey:   {runtime.GOOS},
			},
			expectedPresenceDetectionCallbackHeaders: map[string][]string{
				kolideArchHeaderKey: {runtime.GOARCH},
				kolideOsHeaderKey:   {runtime.GOOS},
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

			// this is the key we can use to open the response
			// it gets set later
			var challengePrivateKey *[32]byte

			callbackWaitGroup := sync.WaitGroup{}

			callbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer callbackWaitGroup.Done()

				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)

				outerResponse := mustUnmarshallOuterResponse(t, mustExtractJsonProperty[string](t, body, "Response"))
				require.Equal(t, challengeId, outerResponse.ChallengeId)

				opened, err := outerResponse.Open(challengePrivateKey)
				require.NoError(t, err)
				require.Equal(t, challengeData, opened.ChallengeData)

				headers := mustExtractJsonProperty[map[string][]string](t, opened.ResponseData, "headers")

				// assume that if we have presence detection header, this is the presence detection callback
				if _, ok := headers[kolideDurationSinceLastPresenceDetectionHeaderKey]; ok {
					require.Equal(t, tt.expectedPresenceDetectionCallbackHeaders, headers,
						"presence detection callback headers should match expected",
					)
					return
				}

				// otherwise it's the regular call back or a "waiting on user callback"
				require.Equal(t, tt.expectedCallbackHeaders, headers,
					"callback headers should match expected",
				)

				w.WriteHeader(http.StatusOK)
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

			requests := []*http.Request{mustMakeGetRequest(t, challengeKryptoBoxB64), mustMakePostRequest(t, challengeKryptoBoxB64)}

			// calculate how many total callbacks we expect

			// by default, even without presence detection, we expect a call back for each request
			expectedCallbacks := len(requests)

			// this is the interval between callbacks for testing
			presenceDetectionCallbackInterval := 750 * time.Millisecond

			// this is how long it will take the "user" to complete presence detection for testing
			simulatedPresenceDetectionCompletionTime := 1 * time.Second

			// assume that if we have presence detection headers, we should have a presence detection callback
			// presence detection is not yet available on linux
			if tt.expectedPresenceDetectionCallbackHeaders != nil && runtime.GOOS != "linux" {

				// we fire off one call back per request immediately when presence detection is starts
				expectedCallbacks += len(requests)

				// calc the number of "waiting on user" callbacks, on an interval well send a call back letting the server know we're still waiting
				expectedCallbacks += int(simulatedPresenceDetectionCompletionTime/presenceDetectionCallbackInterval) * len(requests)

				// we expect one call back per request when the detection is complete
				expectedCallbacks += len(requests)
			}

			callbackWaitGroup.Add(expectedCallbacks)

			for _, req := range requests {
				req := req
				t.Run(req.Method, func(t *testing.T) {
					t.Parallel()

					var logBytes threadsafebuffer.ThreadSafeBuffer
					slogger := multislogger.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
						Level: slog.LevelDebug,
					})).Logger

					mockPresenceDetector := mocks.NewPresenceDetector(t)

					if runtime.GOOS != "linux" { // only doing persence detection on windows and macos for now
						mockPresenceDetector.On("DetectPresence", mock.AnythingOfType("string"), mock.AnythingOfType("Duration")).
							After(simulatedPresenceDetectionCompletionTime).
							Return(0*time.Second, nil).
							Once()
					}

					k := typesmocks.NewKnapsack(t)

					// set up middlewares
					kryptoEcMiddleware := newKryptoEcMiddleware(slogger, k, localServerPrivateKey, remoteServerPrivateKey.PublicKey, mockPresenceDetector, "test-munemo")
					kryptoEcMiddleware.presenceDetectionStatusUpdateInterval = presenceDetectionCallbackInterval

					rr := httptest.NewRecorder()
					// give our middleware with the test handler to the determiner
					kryptoEcMiddleware.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						// make sure all the request headers are present
						for k, v := range cmdReq.Headers {
							require.Equal(t, v[0], r.Header.Get(k))
						}

						w.Write(responseBody)
					})).ServeHTTP(rr, req)

					expecteKolideSessionId := false
					for _, headerMap := range []map[string][]string{tt.cmdReqHeaders, tt.cmdReqCallbackHeaders} {
						_, expecteKolideSessionId = headerMap[kolideSessionIdHeaderKey]
						if expecteKolideSessionId {
							break
						}
					}

					if expecteKolideSessionId {
						require.Contains(t, logBytes.String(), multislogger.KolideSessionIdKey.String())
						require.Contains(t, logBytes.String(), koldieSessionId)
					}

					require.Contains(t, logBytes.String(), multislogger.SpanIdKey.String())
					require.Equal(t, http.StatusOK, rr.Code)
					require.Equal(t, kolideKryptoEccHeader20230130Value, rr.Header().Get(kolideKryptoHeaderKey))

					outerResponse := mustUnmarshallOuterResponse(t, string(rr.Body.Bytes()))
					require.Equal(t, challengeId, outerResponse.ChallengeId)

					opened, err := outerResponse.Open(challengePrivateKey)
					require.NoError(t, err)
					require.Equal(t, challengeData, opened.ChallengeData)

					require.Equal(t, mustExtractJsonProperty[string](t, responseBody, "body"), mustExtractJsonProperty[string](t, opened.ResponseData, "body"),
						"returned response body should match the expected response body",
					)

					require.WithinDuration(t, time.Now(), time.Unix(opened.Timestamp, 0), time.Second*5)

					responseHeaders := mustExtractJsonProperty[map[string][]string](t, opened.ResponseData, "headers")
					require.Equal(t, tt.expectedResponseHeaders, responseHeaders)

					// wait for the callbacks to finish so callback server can run tests
					callbackWaitGroup.Wait()
				})
			}
		})
	}
}

func TestKryptoEcMiddlewareErrors(t *testing.T) {
	t.Parallel()

	// set up keys
	remoteServerPrivateKey := mustGenEcdsaKey(t)
	localServerPrivateKey := mustGenEcdsaKey(t)

	var tests = []struct {
		name          string
		loggedErr     string
		challenge     func() string
		middlewareOpt func(*kryptoEcMiddleware)
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
		{
			name: "timestamp invalid",
			challenge: func() string {
				challenge, _ := mustGenerateChallenge(t, remoteServerPrivateKey, []byte(ulid.New()), []byte(ulid.New()), mustMarshal(t, v2CmdRequestType{}))
				return challenge
			},
			loggedErr: "timestamp is out of range",
			middlewareOpt: func(k *kryptoEcMiddleware) {
				k.timestampValidityRange = -1
			},
		},
		{
			name: "munemo mismatch",
			challenge: func() string {
				challenge, _ := mustGenerateChallenge(t, remoteServerPrivateKey, []byte(ulid.New()), []byte(ulid.New()), mustMarshal(t, v2CmdRequestType{
					Headers: map[string][]string{
						kolideMunemoHeaderKey: {"wrong-munemo"},
					},
				}))
				return challenge
			},
			loggedErr: "munemo mismatch",
			middlewareOpt: func(k *kryptoEcMiddleware) {
				k.timestampValidityRange = -1
			},
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
					t.Parallel()

					var logBytes bytes.Buffer
					slogger := multislogger.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
						Level: slog.LevelDebug,
					})).Logger

					mockPresenceDetector := mocks.NewPresenceDetector(t)
					mockPresenceDetector.On("DetectPresence", mock.AnythingOfType("string"), mock.AnythingOfType("Duration")).Return(0*time.Second, nil).Maybe()

					k := typesmocks.NewKnapsack(t)

					// set up middlewares
					kryptoEcMiddleware := newKryptoEcMiddleware(slogger, k, localServerPrivateKey, remoteServerPrivateKey.PublicKey, mockPresenceDetector, "test-munemo")
					if tt.middlewareOpt != nil {
						tt.middlewareOpt(kryptoEcMiddleware)
					}

					rr := httptest.NewRecorder()

					// give our middleware with the test handler to the determiner
					kryptoEcMiddleware.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Write([]byte("what evs"))
					})).ServeHTTP(rr, req)

					require.Contains(t, logBytes.String(), tt.loggedErr)

					require.NotEqual(t, http.StatusOK, rr.Code,
						"should not have 200 status code on failure",
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

			k := typesmocks.NewKnapsack(t)

			// set up middlewares
			kryptoEcMiddleware := newKryptoEcMiddleware(slogger, k, mustGenEcdsaKey(t), counterpartyKey.PublicKey, mockPresenceDetector, "")

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

			outerRespnse := mustUnmarshallOuterResponse(t, rr.Body.String())
			_, err := outerRespnse.Open(privateEncryptionKey)
			require.NoError(t, err)
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

func mustUnmarshallOuterResponse(t *testing.T, responseB64 string) *challenge.OuterResponse {
	returnedResponseBytes, err := base64.StdEncoding.DecodeString(responseB64)
	require.NoError(t, err)

	responseUnmarshalled, err := challenge.UnmarshalResponse(returnedResponseBytes)
	require.NoError(t, err)
	return responseUnmarshalled
}

func mustMakeGetRequest(t *testing.T, challengeKryptoBoxB64 string) *http.Request {
	v := url.Values{}
	v.Set("box", challengeKryptoBoxB64)

	urlString := fmt.Sprint("https://127.0.0.1:8080?", v.Encode())

	req, err := http.NewRequest(http.MethodGet, urlString, nil) //nolint:noctx // We don't care about this in tests
	require.NoError(t, err)

	return req
}

func mustMakePostRequest(t *testing.T, challengeKryptoBoxB64 string) *http.Request {
	urlString := "https://127.0.0.1:8080"

	body, err := json.Marshal(map[string]string{
		"box": challengeKryptoBoxB64,
	})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, urlString, bytes.NewBuffer(body)) //nolint:noctx // We don't care about this in tests
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

func TestMunemoCheck(t *testing.T) {
	t.Parallel()

	expectedMunemo := "test-munemo"
	validTestHeader := map[string][]string{"kolideMunemoHeaderKey": {expectedMunemo}}

	tests := []struct {
		name                      string
		headers                   map[string][]string
		registrations             []types.Registration
		expectMunemoExtractionErr bool
		expectMiddleWareCheckErr  bool
	}{
		{
			name:    "matching munemo",
			headers: validTestHeader,
			registrations: []types.Registration{
				{
					RegistrationID: types.DefaultRegistrationID,
					Munemo:         expectedMunemo,
				},
			},
		},
		{
			name: "no munemo header",
			registrations: []types.Registration{
				{
					RegistrationID: types.DefaultRegistrationID,
					Munemo:         expectedMunemo,
				},
			},
		},
		{
			name:                      "no registrations",
			headers:                   validTestHeader,
			registrations:             []types.Registration{},
			expectMunemoExtractionErr: true,
		},
		{
			name:    "no default registration",
			headers: validTestHeader,
			registrations: []types.Registration{
				{
					RegistrationID: "some-other-registration-id",
					Munemo:         "some-other-munemo",
				},
			},
			expectMunemoExtractionErr: true,
		},
		{
			name:    "header and munemo dont match",
			headers: map[string][]string{kolideMunemoHeaderKey: {"other-munemo"}},
			registrations: []types.Registration{
				{
					RegistrationID: types.DefaultRegistrationID,
					Munemo:         expectedMunemo,
				},
			},
			expectMiddleWareCheckErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			k := typesmocks.NewKnapsack(t)
			k.On("Registrations").Return(tt.registrations, nil)

			munemo, err := getMunemoFromKnapsack(k)
			if tt.expectMunemoExtractionErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			slogger := multislogger.NewNopLogger()

			e := newKryptoEcMiddleware(slogger, k, nil, mustGenEcdsaKey(t).PublicKey, nil, munemo)
			err = e.checkMunemo(tt.headers)
			if tt.expectMiddleWareCheckErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func Test_sendCallback(t *testing.T) {
	t.Parallel()

	// Set up a test server to receive callback requests
	requestsReceived := &atomic.Int64{}
	testCallbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsReceived.Add(1)
		w.Write([]byte("{}"))
	}))

	// Make sure we close the server at the end of our test
	t.Cleanup(func() {
		testCallbackServer.Close()
	})

	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}))
	k := typesmocks.NewKnapsack(t)

	requestsQueued := &atomic.Int64{}
	mw := newKryptoEcMiddleware(slogger, k, nil, mustGenEcdsaKey(t).PublicKey, nil, "test-munemo")
	for range callbackQueueCapacity {
		go func() {
			req, err := http.NewRequestWithContext(context.TODO(), http.MethodPost, testCallbackServer.URL, nil)
			require.NoError(t, err)
			mw.sendCallback(req, &callbackDataStruct{})
			requestsQueued.Add(1)
		}()
	}

	// Wait a little bit to give the requests a chance to enqueue
	time.Sleep(5 * time.Second)

	// We should have been able to add all requests to the queue
	require.Equal(t, callbackQueueCapacity, int(requestsQueued.Load()), "could not add all requests to queue; logs: ", logBytes.String())

	// We should have sent at least some of them
	require.GreaterOrEqual(t, int(requestsReceived.Load()), maxDesiredCallbackQueueSize, "queue worker did not process expected number of requests; logs: ", logBytes.String())
}

func Test_sendCallback_handlesEnrollment(t *testing.T) {
	t.Parallel()

	// Set up a test server to receive callback requests and return enrollment info
	requestsReceived := &atomic.Int64{}
	expectedNodeKey := "test-node-key"
	expectedMunemo := "test-munemo"
	resp := callbackResponse{
		NodeKey: expectedNodeKey,
		Munemo:  expectedMunemo,
	}
	respRaw, err := json.Marshal(resp)
	require.NoError(t, err)
	testCallbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsReceived.Add(1)
		w.Write(respRaw)
	}))

	// Make sure we close the server at the end of our test
	t.Cleanup(func() {
		testCallbackServer.Close()
	})

	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}))
	k := typesmocks.NewKnapsack(t)
	k.On("SaveRegistration", types.DefaultRegistrationID, expectedMunemo, expectedNodeKey, "").Return(nil)

	mw := newKryptoEcMiddleware(slogger, k, nil, mustGenEcdsaKey(t).PublicKey, nil, "")

	// Confirm we do not have a munemo set
	require.Equal(t, "", mw.tenantMunemo.Load())

	requestsQueued := &atomic.Int64{}
	for range callbackQueueCapacity {
		go func() {
			req, err := http.NewRequestWithContext(context.TODO(), http.MethodPost, testCallbackServer.URL, nil)
			require.NoError(t, err)
			mw.sendCallback(req, &callbackDataStruct{})
			requestsQueued.Add(1)
		}()
	}

	// Wait a little bit to give the requests a chance to enqueue
	time.Sleep(5 * time.Second)

	// We should have been able to add all requests to the queue
	require.Equal(t, callbackQueueCapacity, int(requestsQueued.Load()), "could not add all requests to queue; logs: ", logBytes.String())

	// We should have sent at least some of them
	require.GreaterOrEqual(t, int(requestsReceived.Load()), maxDesiredCallbackQueueSize, "queue worker did not process expected number of requests; logs: ", logBytes.String())

	// We should have called SaveRegistration
	k.AssertExpectations(t)
}
