package localserver

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/kolide/krypto"
	"github.com/kolide/krypto/pkg/challenge"
	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/gowrapper"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/traces"
	"go.opentelemetry.io/otel/attribute"
)

const (
	timestampValidityRange                            = 150
	kolideKryptoEccHeader20230130Value                = "2023-01-30"
	kolideKryptoHeaderKey                             = "X-Kolide-Krypto"
	kolideSessionIdHeaderKey                          = "X-Kolide-Session"
	kolidePresenceDetectionIntervalHeaderKey          = "X-Kolide-Presence-Detection-Interval"
	kolidePresenceDetectionReasonMacosHeaderKey       = "X-Kolide-Presence-Detection-Reason-Macos"
	kolidePresenceDetectionReasonWindowsHeaderKey     = "X-Kolide-Presence-Detection-Reason-Windows"
	kolideDurationSinceLastPresenceDetectionHeaderKey = "X-Kolide-Duration-Since-Last-Presence-Detection"
	kolideOsHeaderKey                                 = "X-Kolide-Os"
	kolideArchHeaderKey                               = "X-Kolide-Arch"
)

type v2CmdRequestType struct {
	Path            string
	Body            []byte
	Headers         map[string][]string
	CallbackUrl     string
	CallbackHeaders map[string][]string
	AllowedOrigins  []string
}

func (cmdReq v2CmdRequestType) CallbackReq() (*http.Request, error) {
	if cmdReq.CallbackUrl == "" {
		return nil, nil
	}

	req, err := http.NewRequest(http.MethodPost, cmdReq.CallbackUrl, nil) //nolint:noctx // Context added in sendCallback()
	if err != nil {
		return nil, fmt.Errorf("making http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Iterate and deep copy
	for h, vals := range cmdReq.CallbackHeaders {
		for _, v := range vals {
			req.Header.Add(h, v)
		}
	}

	return req, nil
}

type kryptoEcMiddleware struct {
	localDbSigner crypto.Signer
	counterParty  ecdsa.PublicKey
	slogger       *slog.Logger
}

func newKryptoEcMiddleware(slogger *slog.Logger, localDbSigner crypto.Signer, counterParty ecdsa.PublicKey) *kryptoEcMiddleware {
	return &kryptoEcMiddleware{
		localDbSigner: localDbSigner,
		counterParty:  counterParty,
		slogger:       slogger.With("keytype", "ec"),
	}
}

// Because callback errors are effectively a shared API with K2, let's define them as a constant and not just
// random strings
type callbackErrors string

const (
	timeOutOfRangeErr   callbackErrors = "time-out-of-range"
	responseFailureErr  callbackErrors = "response-failure"
	originDisallowedErr callbackErrors = "origin-disallowed"
)

type callbackDataStruct struct {
	Time      int64
	Error     callbackErrors
	Response  string // expected base64 encoded krypto box
	UserAgent string
}

// sendCallback is a command to allow launcher to callback to the SaaS side with krypto responses. As the URL it inside
// the signed data, and the response is encrypted, this is reasonably secure.
//
// Also, because the URL is the box, we cannot cleanly do this through middleware. It reqires a lot of passing data
// around through context. Doing it here, as part of kryptoEcMiddleware, allows for a fairly succint defer.
//
// Note that because this is a network call, it should be called in a goroutine.
func (e *kryptoEcMiddleware) sendCallback(req *http.Request, data *callbackDataStruct) {
	if req == nil {
		return
	}

	b, err := json.Marshal(data)
	if err != nil {
		e.slogger.Log(req.Context(), slog.LevelError,
			"unable to marshal callback data",
			"err", err,
		)
	}

	req.Body = io.NopCloser(bytes.NewReader(b))

	// TODO: This feels like it would be cleaner if we passed in an http client at initialzation time
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	ctx, cancel := context.WithTimeout(context.Background(), client.Timeout)
	defer cancel()

	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		e.slogger.Log(req.Context(), slog.LevelError,
			"got error in callback",
			"err", err,
		)
		return
	}

	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}

	e.slogger.Log(req.Context(), slog.LevelDebug,
		"finished callback",
		"response_status", resp.Status,
	)
}

func (e *kryptoEcMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r, span := traces.StartHttpRequestSpan(r)

		defer span.End()

		if r.Body != nil {
			defer r.Body.Close()
		}

		challengeBox, err := extractChallenge(r)
		if err != nil {
			traces.SetError(span, err)
			e.slogger.Log(r.Context(), slog.LevelDebug,
				"failed to extract box from request",
				"err", err,
				"path", r.URL.Path,
				"query_params", r.URL.RawQuery,
			)

			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if err := challengeBox.Verify(e.counterParty); err != nil {
			traces.SetError(span, err)
			e.slogger.Log(r.Context(), slog.LevelDebug,
				"unable to verify signature",
				"err", err,
			)

			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Unmarshal the response _before_ checking the timestamp. This lets us grab the signed callback url to communicate
		// timestamp issues.
		var cmdReq v2CmdRequestType
		if err := json.Unmarshal(challengeBox.RequestData(), &cmdReq); err != nil {
			traces.SetError(span, err)
			e.slogger.Log(r.Context(), slog.LevelError,
				"unable to unmarshal cmd request",
				"err", err,
			)

			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// set the kolide session id if it exists, this also the saml session id
		kolideSessionId, ok := cmdReq.CallbackHeaders[kolideSessionIdHeaderKey]
		if ok && len(kolideSessionId) > 0 {
			span.SetAttributes(attribute.String(kolideSessionIdHeaderKey, kolideSessionId[0]))
			r = r.WithContext(context.WithValue(r.Context(), multislogger.KolideSessionIdKey, kolideSessionId[0]))
		}

		// Setup callback URLs and data. This is a pointer, so it can be adjusted before the defer triggers
		callbackData := &callbackDataStruct{
			Time:      time.Now().Unix(),
			UserAgent: r.Header.Get("User-Agent"),
		}

		if callbackReq, err := cmdReq.CallbackReq(); err != nil {
			e.slogger.Log(r.Context(), slog.LevelError,
				"unable to create callback req",
				"err", err,
			)
		} else if callbackReq != nil {
			defer func() {
				if len(kolideSessionId) > 0 {
					callbackReq = callbackReq.WithContext(
						// adding the current request context will cause this to be cancelled before it sends
						// so just add the session id manually
						context.WithValue(callbackReq.Context(), multislogger.KolideSessionIdKey, kolideSessionId[0]),
					)
				}
				gowrapper.Go(r.Context(), e.slogger, func() {
					e.sendCallback(callbackReq, callbackData)
				})
			}()
		}

		// Check if the origin is in the allowed list. See https://github.com/kolide/k2/issues/9634
		origin := r.Header.Get("Origin")
		// When loading images, the origin may not be set, but the referer will. We can accept that instead.
		if origin == "" {
			origin = strings.TrimSuffix(r.Header.Get("Referer"), "/")
		}
		if len(cmdReq.AllowedOrigins) > 0 {
			allowed := false
			for _, ao := range cmdReq.AllowedOrigins {
				if strings.EqualFold(ao, origin) {
					allowed = true
					break
				}
			}

			if !allowed {
				span.SetAttributes(attribute.String("origin", origin))
				traces.SetError(span, fmt.Errorf("origin %s is not allowed", origin))
				e.slogger.Log(r.Context(), slog.LevelError,
					"origin is not allowed",
					"allowlist", cmdReq.AllowedOrigins,
					"origin", origin,
				)

				w.WriteHeader(http.StatusUnauthorized)
				callbackData.Error = originDisallowedErr
				return
			}

			e.slogger.Log(r.Context(), slog.LevelDebug,
				"origin matches allowlist",
				"origin", origin,
			)
		} else {
			e.slogger.Log(r.Context(), slog.LevelDebug,
				"origin is allowed by default, no allowlist",
				"origin", origin,
			)
		}

		// Check the timestamp, this prevents people from saving a challenge and then
		// reusing it a bunch. However, it will fail if the clocks are too far out of sync.
		timestampDelta := time.Now().Unix() - challengeBox.Timestamp()
		if timestampDelta > timestampValidityRange || timestampDelta < -timestampValidityRange {
			span.SetAttributes(attribute.Int64("timestamp_delta", timestampDelta))
			traces.SetError(span, errors.New("timestamp is out of range"))
			e.slogger.Log(r.Context(), slog.LevelError,
				"timestamp is out of range",
				"delta", timestampDelta,
			)

			w.WriteHeader(http.StatusUnauthorized)
			callbackData.Error = timeOutOfRangeErr
			return
		}

		newReq := &http.Request{
			Method: http.MethodPost,
			Header: make(http.Header),
			URL: &url.URL{
				Scheme: r.URL.Scheme,
				Host:   r.Host,
				Path:   cmdReq.Path,
			},
		}

		for h, vals := range cmdReq.Headers {
			for _, v := range vals {
				newReq.Header.Add(h, v)
			}
		}

		newReq.Header.Set("Origin", r.Header.Get("Origin"))
		newReq.Header.Set("Referer", r.Header.Get("Referer"))

		// setting the newReq context to the current request context
		// allows the trace to continue to the inner request,
		// maintains the same lifetime as the original request,
		// allows same ctx values such as session id to be passed to the inner request
		newReq = newReq.WithContext(r.Context())

		// the body of the cmdReq become the body of the next http request
		if cmdReq.Body != nil && len(cmdReq.Body) > 0 {
			newReq.Body = io.NopCloser(bytes.NewBuffer(cmdReq.Body))
		}

		e.slogger.Log(r.Context(), slog.LevelDebug, "successful challenge, proxying")
		span.AddEvent("challenge_success")

		// bhr contains the data returned by the request defined above
		bhr := &bufferedHttpResponse{}
		next.ServeHTTP(bhr, newReq)

		bhr.Header().Add(kolideOsHeaderKey, runtime.GOOS)
		bhr.Header().Add(kolideArchHeaderKey, runtime.GOARCH)

		// add headers to the response map
		// this assumes that the response to `bhr` was a json encoded blob.
		var responseMap map[string]interface{}
		bhrBytes := bhr.Bytes()
		if err := json.Unmarshal(bhrBytes, &responseMap); err != nil {
			traces.SetError(span, err)
			e.slogger.Log(r.Context(), slog.LevelError,
				"unable to unmarshal response",
				"err", err,
			)
			responseMap = map[string]any{
				"headers": bhr.Header(),

				// the request body was not in json format, just pass it through as "body"
				"body": string(bhrBytes),
			}
		} else {
			responseMap["headers"] = bhr.Header()
		}

		responseBytes, err := json.Marshal(responseMap)
		if err != nil {
			traces.SetError(span, err)
			e.slogger.Log(r.Context(), slog.LevelError,
				"unable to marshal response",
				"err", err,
			)
		}

		var response []byte
		// it's possible the keys will be noop keys, then they will error or give nil when crypto.Signer funcs are called
		// krypto library has a nil check for the object but not the funcs, so if are getting nil from the funcs, just
		// pass nil to krypto
		// hardware signing is not implemented for darwin
		if runtime.GOOS != "darwin" && agent.HardwareKeys() != nil && agent.HardwareKeys().Public() != nil {
			response, err = challengeBox.Respond(e.localDbSigner, agent.HardwareKeys(), responseBytes)
		} else {
			response, err = challengeBox.Respond(e.localDbSigner, nil, responseBytes)
		}

		if err != nil {
			traces.SetError(span, err)
			e.slogger.Log(r.Context(), slog.LevelError,
				"failed to respond",
				"err", err,
			)
			w.WriteHeader(http.StatusUnauthorized)
			callbackData.Error = responseFailureErr
			return
		}

		// because the response is a []byte, we need a copy to prevent simultaneous accessing. Conviniently we can cast
		// it to a string, which has an implicit copy
		callbackData.Response = base64.StdEncoding.EncodeToString(response)

		w.Header().Add(kolideKryptoHeaderKey, kolideKryptoEccHeader20230130Value)

		// arguable the png things here should be their own handler. But doing that means another layer
		// buffering the http response, so it feels a bit silly. When we ditch the v1/v2 switcher, we can
		// be a bit more clever and move this.
		if strings.HasSuffix(cmdReq.Path, ".png") {
			krypto.ToPng(w, response)
		} else {
			w.Write([]byte(base64.StdEncoding.EncodeToString(response)))
		}
	})
}

// extractChallenge finds the challenge in an http request. It prefers the GET parameter, but will fall back to POST data.
func extractChallenge(r *http.Request) (*challenge.OuterChallenge, error) {
	// first check query parmeters
	rawBox := r.URL.Query().Get("box")
	if rawBox != "" {
		decoded, err := base64.StdEncoding.DecodeString(rawBox)
		if err != nil {
			return nil, fmt.Errorf("decoding b64 box from url param: %w", err)
		}

		return challenge.UnmarshalChallenge(decoded)
	}

	// now check body
	if r.Body == nil {
		return nil, fmt.Errorf("no box found in url params or request body: body nil")
	}

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("unmarshalling request body to json: %w", err)
	}

	val, ok := body["box"]
	if !ok {
		return nil, fmt.Errorf("no box key found in request body json")
	}

	valStr, ok := val.(string)
	if !ok {
		return nil, fmt.Errorf("box value is not a string")
	}

	decoded, err := base64.StdEncoding.DecodeString(valStr)
	if err != nil {
		return nil, fmt.Errorf("decoding b64 box from request body: %w", err)
	}

	return challenge.UnmarshalChallenge(decoded)
}

type bufferedHttpResponse struct {
	header http.Header
	code   int
	buf    bytes.Buffer
}

func (bhr *bufferedHttpResponse) Header() http.Header {
	if bhr.header == nil {
		bhr.header = make(http.Header)
	}

	return bhr.header
}

func (bhr *bufferedHttpResponse) Write(in []byte) (int, error) {
	return bhr.buf.Write(in)
}

func (bhr *bufferedHttpResponse) WriteHeader(code int) {
	bhr.code = code
}

func (bhr *bufferedHttpResponse) Bytes() []byte {
	return bhr.buf.Bytes()
}
