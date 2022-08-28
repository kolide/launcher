package localserver

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/go-kit/kit/log/level"
)

type closingBuffer struct {
	*bytes.Buffer
}

func (cb closingBuffer) Close() error { return nil }

const (
	v0CmdHeader    = "X-K2-Cmd"
	v0CmdSignature = "X-K2-Signature"
)

// UnwrapV0 is a middleware that will authenticate and decode the requests from headers, and then pass them on to
// the next handler.This completely ignore the request body.
func (ls *localServer) UnwrapV0Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body.Close()
		}

		cmds, ok := r.Header[v0CmdHeader]
		if !ok || len(cmds) == 0 || len(cmds[0]) == 0 {
			level.Debug(ls.logger).Log("msg", "No command in request")
			w.WriteHeader(401)
			return
		}
		cmd := cmds[0]

		sigs, ok := r.Header[v0CmdSignature]
		if !ok || len(sigs) == 0 || len(sigs[0]) == 0 {
			level.Debug(ls.logger).Log("msg", "No signature in request")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		signature, err := base64.StdEncoding.DecodeString(sigs[0])
		if err != nil {
			level.Debug(ls.logger).Log("msg", "unable to decode signature", "err", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if err := ls.verify([]byte(cmd), signature); err != nil {
			level.Debug(ls.logger).Log("msg", "signature mismatch", "err", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		newReq := &http.Request{
			Method: "GET",
			URL:    &url.URL{Path: cmd},
		}

		next.ServeHTTP(w, newReq)
	})
}

func (krw *kryptoBoxResponseWriter) UnwrapVX(next http.Handler) http.Handler {
	// TODO maybe check max body size before we do this? Or implement something streaming.
	// On the other hand, the requests are coming from localhost...
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		body, err := io.ReadAll(r.Body)
		if err != nil {
			fmt.Printf("Failed to io.Copy: %v", err)
			w.WriteHeader(401)
			return
		} else if len(body) == 0 {
			fmt.Println("No data in request", err)
			w.WriteHeader(401)
			return
		}

		decoded, err := krw.boxer.DecodeRaw(body)
		if err != nil {
			//level.Debug(ls.logger).Log("msg", "Unable to decode request", "err", err)
			fmt.Println("Unable to decode request", err)
			w.WriteHeader(401)
			return
		}

		r.Body = closingBuffer{bytes.NewBuffer(decoded.Data())}
		next.ServeHTTP(w, r)
	})
}
