package localserver

import (
	"crypto"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
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
			r.Body.Close()
		}

		// Extract the box from the URL query parameters
		boxRaw := r.URL.Query().Get("box")
		if boxRaw == "" {
			level.Debug(e.logger).Log("msg", "no data in box query parameter")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		boxRawBytes, err := base64.StdEncoding.DecodeString(boxRaw)
		if err != nil {
			level.Debug(e.logger).Log("msg", "failed to b64 decode box", "err", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		challengeBox, err := challenge.UnmarshalChallenge(boxRawBytes)
		if err != nil {
			level.Debug(e.logger).Log("msg", "failed to unmarshal challenge", "err", err)
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

		v := url.Values{}

		newReq := &http.Request{
			Method: "GET",
			URL: &url.URL{
				Scheme:   r.URL.Scheme,
				Host:     r.Host,
				Path:     cmdReq.Path,
				RawQuery: v.Encode(),
			},
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
