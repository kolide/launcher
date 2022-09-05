package localserver

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/go-kit/kit/log/level"
)

type cmdRequestType struct {
	Cmd string
	Id  string
}

// Unwrapv1 is middleware that ingests a krypto.Box from the GET requests, and after verifying the signature, converts
// it to a new http request and passes it to the next handler. (This is all coming in via the URL, because that's a
//
//	limitation we have from js)
func (kbm *kryptoBoxerMiddleware) UnwrapV1Hander(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body.Close()
		}

		// Extract the box from the URL query parameters
		boxRaw := r.URL.Query().Get("box")
		if boxRaw == "" {
			level.Debug(kbm.logger).Log("msg", "no data in box query parameter")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		box, err := base64.StdEncoding.DecodeString(boxRaw)
		if err != nil {
			level.Debug(kbm.logger).Log("msg", "unable to base64 decode box", "err", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		decoded, err := kbm.boxer.DecodeRaw(box)
		if err != nil {
			level.Debug(kbm.logger).Log("msg", "unable to verify box", "err", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if decoded == nil {
			level.Debug(kbm.logger).Log("msg", "nil box", "err", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var cmdReq cmdRequestType
		if err := json.Unmarshal(decoded.Signedtext, &cmdReq); err != nil {
			level.Debug(kbm.logger).Log("msg", "unable to unmarshal cmd request", "err", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		v := url.Values{}

		if cmdReq.Id != "" {
			v.Set("id", cmdReq.Id)
		}

		newReq := &http.Request{
			Method: "GET",
			URL: &url.URL{
				Scheme:   r.URL.Scheme,
				Host:     r.Host,
				Path:     cmdReq.Cmd,
				RawQuery: v.Encode(),
			},
		}

		next.ServeHTTP(w, newReq)
	})
}
