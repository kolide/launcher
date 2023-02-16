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

		// Check to see if we have a body
		if r.Body != nil {
			//posts were added after v1 (rsaKrypto) was, so it must be v2 (ecMiddleware) request
			h.ecMiddleware.ServeHTTP(w, r)
			return
		}

		level.Debug(h.logger).Log("msg", "no data in box query parameter or body")
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
