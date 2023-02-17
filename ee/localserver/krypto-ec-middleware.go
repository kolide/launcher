package localserver

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/krypto"
	"github.com/kolide/krypto/pkg/challenge"
)

const (
	timestampValidityRange             = 150
	kolideKryptoEccHeader20230130Value = "2023-01-30"
	kolideKryptoHeaderKey              = "X-Kolide-Krypto"
)

type v2CmdRequestType struct {
	Path string
	Body []byte
}

type kryptoEcMiddleware struct {
	localDbSigner, hardwareSigner crypto.Signer
	counterParty                  ecdsa.PublicKey
	logger                        log.Logger
}

func newKryptoEcMiddleware(logger log.Logger, localDbSigner, hardwareSigner crypto.Signer, counterParty ecdsa.PublicKey) *kryptoEcMiddleware {
	return &kryptoEcMiddleware{
		localDbSigner:  localDbSigner,
		hardwareSigner: hardwareSigner,
		counterParty:   counterParty,
		logger:         log.With(logger, "keytype", "ec"),
	}
}

func (e *kryptoEcMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			defer r.Body.Close()
		}

		challengeBox, err := extractChallenge(r)
		if err != nil {
			level.Debug(e.logger).Log("msg", "failed to extract box from request", "err", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if err := challengeBox.Verify(e.counterParty); err != nil {
			level.Debug(e.logger).Log("msg", "unable to verify signature", "err", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Check the timestamp, this prevents people from saving a challenge and then
		// reusing it a bunch. However, it will fail if the clocks are too far out of sync.
		timestampDelta := time.Now().Unix() - challengeBox.Timestamp()
		if timestampDelta > timestampValidityRange || timestampDelta < -timestampValidityRange {
			level.Debug(e.logger).Log("msg", "timestamp is out of range", "delta", timestampDelta)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var cmdReq v2CmdRequestType
		if err := json.Unmarshal(challengeBox.RequestData(), &cmdReq); err != nil {
			level.Debug(e.logger).Log("msg", "unable to unmarshal cmd request", "err", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		newReq := &http.Request{
			Method: http.MethodPost,
			URL: &url.URL{
				Scheme: r.URL.Scheme,
				Host:   r.Host,
				Path:   cmdReq.Path,
			},
		}

		// the body of the cmdReq become the body of the next http request
		if cmdReq.Body != nil && len(cmdReq.Body) > 0 {
			newReq.Body = io.NopCloser(bytes.NewBuffer(cmdReq.Body))
		}

		level.Debug(e.logger).Log("msg", "Successful challenge. Proxying")

		// bhr contains the data returned by the request defined above
		bhr := &bufferedHttpResponse{}
		next.ServeHTTP(bhr, newReq)

		var response []byte
		// it's possible the keys will be noop keys, then they will error or give nil when crypto.Signer funcs are called
		// krypto library has a nil check for the object but not the funcs, so if are getting nil from the funcs, just
		// pass nil to krypto
		if e.hardwareSigner != nil && e.hardwareSigner.Public() != nil {
			response, err = challengeBox.Respond(e.localDbSigner, e.hardwareSigner, bhr.Bytes())
		} else {
			response, err = challengeBox.Respond(e.localDbSigner, nil, bhr.Bytes())
		}

		if err != nil {
			level.Debug(e.logger).Log("msg", "failed to respond", "err", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

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
