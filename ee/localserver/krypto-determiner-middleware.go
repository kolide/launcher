package localserver

import (
	"encoding/base64"
	"net/http"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/vmihailenco/msgpack/v5"
)

type kryptoDeterminerMiddleware struct {
	rsaMiddleware http.Handler
	ecMiddleware  http.Handler
	logger        log.Logger
}

func NewKryptoDeterminerMiddleware(logger log.Logger, rsaMiddleware http.Handler, ecMiddleware http.Handler) *kryptoDeterminerMiddleware {
	return &kryptoDeterminerMiddleware{
		rsaMiddleware: rsaMiddleware,
		ecMiddleware:  ecMiddleware,
		logger:        logger,
	}
}

func (h *kryptoDeterminerMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract the box from the URL query parameters
	boxRaw := r.URL.Query().Get("box")
	if boxRaw == "" {
		level.Debug(h.logger).Log("msg", "no data in box query parameter")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	box, err := base64.StdEncoding.DecodeString(boxRaw)
	if err != nil {
		level.Debug(h.logger).Log("msg", "unable to base64 decode box", "err", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Playing with msgpack, it will unmarshal to map[string]any, which makes it
	// simpler enough for us to handle routing here.
	var boxMap map[string]interface{}
	if err := msgpack.Unmarshal(box, &boxMap); err != nil {
		level.Debug(h.logger).Log("msg", "unable to unmarshal box", "err", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// if it's got the Sig key, then it's a v2 box
	if _, ok := boxMap["sig"]; ok {
		h.ecMiddleware.ServeHTTP(w, r)
		return
	}

	// Signature key is v1
	if _, ok := boxMap["signature"]; ok {
		h.rsaMiddleware.ServeHTTP(w, r)
		return
	}

	// Eh, who knows
	level.Debug(h.logger).Log("msg", "Unknown box type")
	w.WriteHeader(http.StatusUnauthorized)
}

// func (h *kryptoDeterminerMiddleware) determineKryptoWrap(next http.Handler) http.Handler {
// 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		if isEcKryptoType(r.URL.Query()) {
// 			h.ecMiddleware.wrapHandler(next).ServeHTTP(w, r)
// 			return
// 		}

// 		h.rsaMiddleware.Wrap(next).ServeHTTP(w, r)
// 		return
// 	})
// }

// func (h *kryptoDeterminerMiddleware) determineKryptoWrapPng(next http.Handler) http.Handler {
// 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		if isEcKryptoType(r.URL.Query()) {
// 			h.ecMiddleware.wrapPngHandler(next).ServeHTTP(w, r)
// 			return
// 		}

// 		h.rsaMiddleware.WrapPng(next).ServeHTTP(w, r)
// 		return
// 	})
// }

// func (e *kryptoEcMiddleware) wrap(next http.Handler, r *http.Request, w http.ResponseWriter, toPng bool) (string, error) {
// 	bhr := &bufferedHttpResponse{}
// 	next.ServeHTTP(bhr, r)

// 	boxRaw := r.URL.Query().Get("box")
// 	if boxRaw == "" {
// 		return "", fmt.Errorf("no data in box query parameter")
// 	}

// 	challengeBytes, err := base64.StdEncoding.DecodeString(boxRaw)
// 	if err != nil {
// 		return "", err
// 	}

// 	challengeBox, err := challenge.UnmarshalChallenge(challengeBytes)
// 	if err != nil {
// 		return "", fmt.Errorf("marshaling outer challenge: %w", err)
// 	}

// 	var responseBytes []byte
// 	switch toPng {
// 	case true:
// 		responseBytes, err = challengeBox.RespondPng(e.signer, bhr.Bytes())
// 		if err != nil {
// 			return "", fmt.Errorf("failed to create challenge response to png: %w", err)
// 		}

// 	case false:
// 		responseBytes, err = challengeBox.Respond(e.signer, bhr.Bytes())
// 		if err != nil {
// 			return "", fmt.Errorf("failed to create challenge response: %w", err)
// 		}
// 	}

// 	return base64.StdEncoding.EncodeToString(responseBytes), nil
// }

// func isEcKryptoType(urlValues url.Values) bool {
// 	kryptoType := urlValues.Get("krypto-type")
// 	if kryptoType == "ec" {
// 		return true
// 	}
// 	return false
// }
