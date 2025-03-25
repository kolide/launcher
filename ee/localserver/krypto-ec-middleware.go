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
	"sync"
	"time"

	"github.com/kolide/krypto"
	"github.com/kolide/krypto/pkg/challenge"
	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/gowrapper"
	"github.com/kolide/launcher/ee/presencedetection"
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
	kolideMunemoHeaderKey                             = "X-Kolide-Munemo"
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
	localDbSigner         crypto.Signer
	counterParty          ecdsa.PublicKey
	slogger               *slog.Logger
	presenceDetector      presenceDetector
	presenceDetectionLock sync.Mutex
	tenantMunemo          string

	// presenceDetectionStatusUpdateInterval is the interval at which the presence detection
	// callback is sent while waiting on user to complete presence detection
	presenceDetectionStatusUpdateInterval time.Duration
	timestampValidityRange                int64
}

func newKryptoEcMiddleware(slogger *slog.Logger, localDbSigner crypto.Signer, counterParty ecdsa.PublicKey, presenceDetector presenceDetector, tenantMunemo string) *kryptoEcMiddleware {
	return &kryptoEcMiddleware{
		localDbSigner:                         localDbSigner,
		counterParty:                          counterParty,
		slogger:                               slogger.With("keytype", "ec"),
		presenceDetector:                      presenceDetector,
		timestampValidityRange:                timestampValidityRange,
		presenceDetectionStatusUpdateInterval: 30 * time.Second,
		tenantMunemo:                          tenantMunemo,
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
func sendCallback(slogger *slog.Logger, req *http.Request, data *callbackDataStruct) {
	if req == nil {
		return
	}

	b, err := json.Marshal(data)
	if err != nil {
		slogger.Log(req.Context(), slog.LevelError,
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
		slogger.Log(req.Context(), slog.LevelError,
			"got error in callback",
			"err", err,
		)
		return
	}

	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}

	slogger.Log(req.Context(), slog.LevelDebug,
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

		// check to see if munemos match
		if err := e.checkMunemo(cmdReq.Headers); err != nil {
			span.AddEvent("munemo_mismatch")
			e.slogger.Log(r.Context(), slog.LevelInfo,
				"munemo mismatch",
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
					sendCallback(e.slogger, callbackReq, callbackData)
				})

				gowrapper.Go(r.Context(), e.slogger, func() {
					e.detectPresence(challengeBox)
				})
			}()
		}

		if err := e.checkOrigin(r, cmdReq.AllowedOrigins); err != nil {
			callbackData.Error = callbackErrors(err.Error())
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if err := e.checkTimestamp(r, challengeBox); err != nil {
			callbackData.Error = callbackErrors(err.Error())
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		newReq := cmdReqToHttpReq(r, cmdReq)

		e.slogger.Log(r.Context(), slog.LevelDebug, "successful challenge, proxying")
		span.AddEvent("challenge_success")

		// bhr contains the data returned by the request defined above
		bhr := &bufferedHttpResponse{}
		next.ServeHTTP(bhr, newReq)

		if len(kolideSessionId) > 0 {
			bhr.Header().Add(kolideSessionIdHeaderKey, kolideSessionId[0])
		}

		bhr.Header().Add(kolideOsHeaderKey, runtime.GOOS)
		bhr.Header().Add(kolideArchHeaderKey, runtime.GOARCH)

		response, err := e.bufferedHttpResponseToKryptoResponse(challengeBox, bhr)
		if err != nil {
			traces.SetError(span, err)
			e.slogger.Log(r.Context(), slog.LevelError,
				"error creating krypto response",
				"err", err,
			)
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
		return nil, errors.New("no box found in url params or request body: body nil")
	}

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("unmarshalling request body to json: %w", err)
	}

	val, ok := body["box"]
	if !ok {
		return nil, errors.New("no box key found in request body json")
	}

	valStr, ok := val.(string)
	if !ok {
		return nil, errors.New("box value is not a string")
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

// detectPresence prompts the user for presence detection and sends periodic status updates to
// k2 (waiting , complete, error, timeout)
func (e *kryptoEcMiddleware) detectPresence(challengeBox *challenge.OuterChallenge) {
	// extract cmd req from challenge box
	ctx, cancel := context.WithTimeout(context.Background(), presencedetection.DetectionTimeout)
	defer cancel()

	// presence detection is not yet available on linux
	if runtime.GOOS == "linux" {
		return
	}

	var cmdReq v2CmdRequestType
	if err := json.Unmarshal(challengeBox.RequestData(), &cmdReq); err != nil {
		e.slogger.Log(ctx, slog.LevelError,
			"error unmarshaling cmd request",
			"err", err,
		)
		return
	}

	// figure out if we need to do presence detection
	detectionIntervalStr, ok := cmdReq.Headers[kolidePresenceDetectionIntervalHeaderKey]
	if !ok || len(detectionIntervalStr) == 0 || detectionIntervalStr[0] == "" {
		return
	}

	detectionIntervalDuration, err := time.ParseDuration(detectionIntervalStr[0])
	if err != nil {
		e.slogger.Log(ctx, slog.LevelError,
			"error parsing presence detection interval",
			"err", err,
		)
		return
	}

	// On MacOS presence detection appears as "Kolide is trying to {reason}."
	reason := "authenticate"

	reasonKey := kolidePresenceDetectionReasonMacosHeaderKey
	if runtime.GOOS == "windows" {
		// On windows presence detection text is expected to be a full sentence
		reason = "Kolide is requesting authentication"
		reasonKey = kolidePresenceDetectionReasonWindowsHeaderKey
	}

	if reasonStr, ok := cmdReq.CallbackHeaders[reasonKey]; ok && len(reasonStr) > 0 && len(reasonStr[0]) > 0 {
		reason = reasonStr[0]
	}

	if !e.presenceDetectionLock.TryLock() {
		e.slogger.Log(ctx, slog.LevelInfo,
			"dropping presence detection callback, already in progress",
		)
		return
	}
	defer e.presenceDetectionLock.Unlock()

	type detectionResult struct {
		durationSinceLastDetection time.Duration
		err                        error
	}

	presenceDetectionStartTime := time.Now()

	detectionDoneChan := make(chan *detectionResult)

	// kick of presence detection
	gowrapper.Go(ctx, e.slogger, func() {
		durationSinceLastDetection, err := e.presenceDetector.DetectPresence(reason, detectionIntervalDuration)
		if err != nil {
			e.slogger.Log(ctx, slog.LevelInfo,
				"presence_detection",
				"reason", reason,
				"interval", detectionIntervalDuration,
				"duration_since_last_detection", durationSinceLastDetection,
				"err", err,
			)
		}

		detectionDoneChan <- &detectionResult{
			durationSinceLastDetection: durationSinceLastDetection,
			err:                        err,
		}
	})

	statusUpdateTicker := time.NewTicker(e.presenceDetectionStatusUpdateInterval)
	defer statusUpdateTicker.Stop()

	headers := map[string][]string{
		kolideArchHeaderKey: {runtime.GOARCH},
		kolideOsHeaderKey:   {runtime.GOOS},
	}

	kolideSessionId, ok := cmdReq.CallbackHeaders[kolideSessionIdHeaderKey]
	if ok {
		headers[kolideSessionIdHeaderKey] = kolideSessionId
	}

	type presenceDetectionMessages string

	const (
		presenceDetectionCompleted presenceDetectionMessages = "presence_detection_complete"
		presenceDetectionTimedOut  presenceDetectionMessages = "presence_detection_timed_out"
		presenceDetectionWaiting   presenceDetectionMessages = "presence_detection_waiting"
		presenceDetectionError     presenceDetectionMessages = "presence_detection_error"
	)

	var finalPresenceDetectionResult *detectionResult
	hasPresenceDetectionTimedout := false

	// send status updates to k2 on an interval until presence detection completes
	for {

		// Build status update for k2
		callbackBody := map[string]string{
			"elapsed_time": time.Since(presenceDetectionStartTime).String(),
		}

		switch {

		// completed with error
		case finalPresenceDetectionResult != nil && finalPresenceDetectionResult.err != nil:
			e.slogger.Log(ctx, slog.LevelWarn,
				"presence detection error",
				"err", finalPresenceDetectionResult.err,
			)

			callbackBody["msg"] = string(presenceDetectionError)
			callbackBody[kolideDurationSinceLastPresenceDetectionHeaderKey] = finalPresenceDetectionResult.durationSinceLastDetection.String()

		// completed without error
		case finalPresenceDetectionResult != nil:
			callbackBody["msg"] = string(presenceDetectionCompleted)
			callbackBody[kolideDurationSinceLastPresenceDetectionHeaderKey] = finalPresenceDetectionResult.durationSinceLastDetection.String()

		// timeout
		case hasPresenceDetectionTimedout:
			callbackBody["msg"] = string(presenceDetectionTimedOut)

		// still waiting on user
		default:
			callbackBody["msg"] = string(presenceDetectionWaiting)
		}

		callBackData, err := e.presenceDetectionCallbackKryptoResponse(challengeBox, headers, callbackBody)
		if err != nil {
			e.slogger.Log(ctx, slog.LevelError,
				"error creating presence detection callback response",
				"err", err,
			)
			return
		}

		req, err := cmdReq.CallbackReq()
		if err != nil {
			e.slogger.Log(ctx, slog.LevelError,
				"error creating presence detection callback request",
				"err", err,
			)
			return
		}

		sendCallback(e.slogger, req, callBackData)

		if finalPresenceDetectionResult != nil || hasPresenceDetectionTimedout {
			return
		}

		select {
		case <-ctx.Done():
			hasPresenceDetectionTimedout = true
			e.slogger.Log(ctx, slog.LevelInfo,
				"presence detection timed out",
				"elapsed_time", time.Since(presenceDetectionStartTime),
				"timeout", presencedetection.DetectionTimeout,
			)
			continue
		case finalPresenceDetectionResult = <-detectionDoneChan:
			headers[kolideDurationSinceLastPresenceDetectionHeaderKey] = []string{finalPresenceDetectionResult.durationSinceLastDetection.String()}
			continue
		case <-statusUpdateTicker.C:
			continue
		}
	}
}

// presenceDetectionCallbackKryptoResponse takes a challenge box, headers, and body objects. Creates an encrypted response and returns in
// the callbackDataStruct expected for callbacks
func (e *kryptoEcMiddleware) presenceDetectionCallbackKryptoResponse(challengeBox *challenge.OuterChallenge, headers map[string][]string, body map[string]string) (*callbackDataStruct, error) {
	data := map[string]any{
		"headers": headers,
		"body":    body,
	}

	responseBytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("error marshaling response: %w", err)
	}

	response, err := e.kryptoResponse(challengeBox, responseBytes)
	if err != nil {
		return nil, fmt.Errorf("error creating krypto response: %w", err)
	}

	return &callbackDataStruct{
		Time:     time.Now().Unix(),
		Response: base64.StdEncoding.EncodeToString(response),
	}, nil
}

// bufferedHttpResponseToResponseData takes a bufferedHttpResponse returns a json blob in the format
// expected (body & headers)
func bufferedHttpResponseToResponseData(bhr *bufferedHttpResponse) ([]byte, error) {
	// add headers to the response map
	// this assumes that the response to `bhr` was a json encoded blob.
	var responseMap map[string]interface{}
	bhrBytes := bhr.Bytes()
	if err := json.Unmarshal(bhrBytes, &responseMap); err != nil {
		responseMap = map[string]any{
			"headers": bhr.Header(),

			// the request body was not in json format, just pass it through as "body"
			"body": string(bhrBytes),
		}
	} else {
		responseMap["headers"] = bhr.Header()
	}

	return json.Marshal(responseMap)
}

// bufferedHttpResponseToKryptoResponse takes a bufferedHttpResponse and a challenge and returns a krypto response
func (e *kryptoEcMiddleware) bufferedHttpResponseToKryptoResponse(box *challenge.OuterChallenge, bhr *bufferedHttpResponse) ([]byte, error) {
	responseBytes, err := bufferedHttpResponseToResponseData(bhr)
	if err != nil {
		return nil, err
	}

	return e.kryptoResponse(box, responseBytes)
}

// kryptoResponse uses provided challenge and data to create a krypto response
func (e *kryptoEcMiddleware) kryptoResponse(box *challenge.OuterChallenge, data []byte) ([]byte, error) {
	var response []byte
	// it's possible the keys will be noop keys, then they will error or give nil when crypto.Signer funcs are called
	// krypto library has a nil check for the object but not the funcs, so if are getting nil from the funcs, just
	// pass nil to krypto
	// hardware signing is not implemented for darwin
	var err error
	if runtime.GOOS != "darwin" && agent.HardwareKeys() != nil && agent.HardwareKeys().Public() != nil {
		response, err = box.Respond(e.localDbSigner, agent.HardwareKeys(), data)
	} else {
		response, err = box.Respond(e.localDbSigner, nil, data)
	}

	if err != nil {
		return nil, err
	}

	return response, nil
}

// checkOrigin checks the origin of the request against a list of allowed origins.
// If no allowlist is provided, all origins are allowed.
func (e *kryptoEcMiddleware) checkOrigin(r *http.Request, allowedOrigins []string) error {
	r, span := traces.StartHttpRequestSpan(r)
	defer span.End()

	// Check if the origin is in the allowed list. See https://github.com/kolide/k2/issues/9634
	origin := r.Header.Get("Origin")
	// When loading images, the origin may not be set, but the referer will. We can accept that instead.
	if origin == "" {
		origin = strings.TrimSuffix(r.Header.Get("Referer"), "/")
	}

	if allowedOrigins == nil || len(allowedOrigins) == 0 {
		e.slogger.Log(r.Context(), slog.LevelDebug,
			"origin is allowed by default, no allowlist",
			"origin", origin,
		)
		return nil
	}

	for _, ao := range allowedOrigins {
		if strings.EqualFold(ao, origin) {
			e.slogger.Log(r.Context(), slog.LevelDebug,
				"origin matches allowlist",
				"origin", origin,
			)

			return nil
		}
	}

	span.SetAttributes(attribute.String("origin", origin))
	traces.SetError(span, fmt.Errorf("origin %s is not allowed", origin))
	e.slogger.Log(r.Context(), slog.LevelError,
		"origin is not allowed",
		"allowlist", allowedOrigins,
		"origin", origin,
	)

	return errors.New(string(originDisallowedErr))
}

// checkTimestamp checks the timestamp of the challenge is within a defined interval.
// This prevents people from saving a challenge and then reusing it a bunch.
// However, it will fail if the clocks are too far out of sync.
func (e *kryptoEcMiddleware) checkTimestamp(r *http.Request, challengeBox *challenge.OuterChallenge) error {
	r, span := traces.StartHttpRequestSpan(r)
	defer span.End()

	// Check the timestamp, this prevents people from saving a challenge and then
	// reusing it a bunch. However, it will fail if the clocks are too far out of sync.
	timestampDelta := time.Now().Unix() - challengeBox.Timestamp()
	if timestampDelta > e.timestampValidityRange || timestampDelta < -e.timestampValidityRange {
		span.SetAttributes(attribute.Int64("timestamp_delta", timestampDelta))
		traces.SetError(span, errors.New("timestamp is out of range"))
		e.slogger.Log(r.Context(), slog.LevelError,
			"timestamp is out of range",
			"delta", timestampDelta,
		)

		return errors.New(string(timeOutOfRangeErr))
	}

	return nil
}

// cmdReqToHttpReq takes the original request and the cmd request and returns a new http.Request
// suitable for passing to standard http handlers
func cmdReqToHttpReq(originalRequest *http.Request, cmdReq v2CmdRequestType) *http.Request {
	newReq := &http.Request{
		Method: http.MethodPost,
		Header: make(http.Header),
		URL: &url.URL{
			Scheme: originalRequest.URL.Scheme,
			Host:   originalRequest.Host,
			Path:   cmdReq.Path,
		},
	}

	for h, vals := range cmdReq.Headers {
		for _, v := range vals {
			newReq.Header.Add(h, v)
		}
	}

	newReq.Header.Set("Origin", originalRequest.Header.Get("Origin"))
	newReq.Header.Set("Referer", originalRequest.Header.Get("Referer"))

	// setting the newReq context to the current request context
	// allows the trace to continue to the inner request,
	// maintains the same lifetime as the original request,
	// allows same ctx values such as session id to be passed to the inner request
	newReq = newReq.WithContext(originalRequest.Context())

	// the body of the cmdReq become the body of the next http request
	if cmdReq.Body != nil && len(cmdReq.Body) > 0 {
		newReq.Body = io.NopCloser(bytes.NewBuffer(cmdReq.Body))
	}

	return newReq
}

func (e *kryptoEcMiddleware) checkMunemo(headers map[string][]string) error {
	if e.tenantMunemo == "" {
		e.slogger.Log(context.TODO(), slog.LevelError,
			"no munemo set in krypto middleware, continuing",
		)
		return nil
	}

	munemoHeaders, ok := headers[kolideMunemoHeaderKey]
	if !ok || len(munemoHeaders) == 0 || munemoHeaders[0] == "" {
		e.slogger.Log(context.TODO(), slog.LevelDebug,
			"no munemo header in request, continuing",
		)
		return nil
	}

	if munemoHeaders[0] == e.tenantMunemo {
		e.slogger.Log(context.TODO(), slog.LevelDebug,
			"munemo in request matches munemo in enroll secret, continuing",
		)
		return nil
	}

	return errors.New("munemo in request does not match munemo in middleware")
}
