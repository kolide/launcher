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
		// If we don't have a box param, assume it's a post. As posts
		// were added after v1 (rsaKrypto) was depreciated, we can send it to  v2 (ecMiddleware).
		// The v2 middleware will return an error if there's no data
		h.ecMiddleware.ServeHTTP(w, r)
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
