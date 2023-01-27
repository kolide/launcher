package localserver

import (
	"encoding/base64"
	"net/http"
	"net/url"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/krypto/pkg/challenge"
	"github.com/vmihailenco/msgpack/v5"
)

type kryptoDeterminerMiddleware struct {
	rsaMiddleware *kryptoBoxerMiddleware
	ecMiddleware  *kryptoEcMiddleware
	logger        log.Logger
}

func NewKryptoDeterminerMiddleware(logger log.Logger, rsaMiddleware *kryptoBoxerMiddleware, ecMiddleware *kryptoEcMiddleware) *kryptoDeterminerMiddleware {
	return &kryptoDeterminerMiddleware{
		rsaMiddleware: rsaMiddleware,
		ecMiddleware:  ecMiddleware,
		logger:        logger,
	}
}

func (h *kryptoDeterminerMiddleware) determineKryptoUnwrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body.Close()
		}

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

		var outerChallenge challenge.OuterChallenge
		if err := msgpack.Unmarshal(box, &outerChallenge); err != nil {
			level.Debug(h.logger).Log("msg", "unable to verify box", "err", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// if we have these 2 fields, assume it's ec krypto
		if outerChallenge.Sig != nil && outerChallenge.Msg != nil {
			h.ecMiddleware.unwrap(next).ServeHTTP(w, r)
			return
		}

		h.rsaMiddleware.UnwrapV1Hander(next).ServeHTTP(w, r)
	})
}

func (h *kryptoDeterminerMiddleware) determineKryptoWrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isEcKryptoType(r.URL.Query()) {
			h.ecMiddleware.wrap(next).ServeHTTP(w, r)
			return
		}

		h.rsaMiddleware.Wrap(next).ServeHTTP(w, r)
		return
	})
}

func (h *kryptoDeterminerMiddleware) determineKryptoWrapPng(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isEcKryptoType(r.URL.Query()) {
			h.ecMiddleware.wrapPng(next).ServeHTTP(w, r)
			return
		}

		h.rsaMiddleware.WrapPng(next).ServeHTTP(w, r)
		return
	})
}

func isEcKryptoType(urlValues url.Values) bool {
	kryptoType := urlValues.Get("krypto-type")
	if kryptoType == "ec" {
		return true
	}
	return false
}
