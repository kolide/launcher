package localserver

import (
	"crypto"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/krypto/pkg/challenge"
	"github.com/kolide/krypto/pkg/echelper"
	"github.com/vmihailenco/msgpack/v5"
)

type kryptoEcMiddleware struct {
	signer       crypto.Signer
	counterParty ecdsa.PublicKey
	logger       log.Logger
}

func newKryptoEcMiddleware(logger log.Logger, signer crypto.Signer, counteParty ecdsa.PublicKey) *kryptoEcMiddleware {
	return &kryptoEcMiddleware{
		signer:       signer,
		counterParty: counteParty,
		logger:       logger,
	}
}

func (e *kryptoEcMiddleware) unwrap(next http.Handler) http.Handler {
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

		outerChallenge, err := marshalOuterChallenge(boxRaw)
		if err != nil {
			level.Debug(e.logger).Log("msg", "failed to marshal outer challenge", "err", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if err := echelper.VerifySignature(e.counterParty, outerChallenge.Msg, outerChallenge.Sig); err != nil {
			level.Debug(e.logger).Log("msg", "unable to verify signature", "err", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var innerChallenge challenge.InnerChallenge
		if err := msgpack.Unmarshal(outerChallenge.Msg, &innerChallenge); err != nil {
			level.Debug(e.logger).Log("msg", "unable to marshall challenge.InnerChallenge", "err", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var cmdReq cmdRequestType
		if err := json.Unmarshal(innerChallenge.ChallengeData, &cmdReq); err != nil {
			level.Debug(e.logger).Log("msg", "unable to unmarshal cmd request", "err", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		v := url.Values{}

		if cmdReq.Id != "" {
			v.Set("id", cmdReq.Id)
		}

		v.Set("box", boxRaw)
		v.Set("krypto-type", "ec")

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

func (e *kryptoEcMiddleware) wrapPng(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, err := e.wrapImpl(next, r, w, true)
		if err != nil {
			level.Debug(e.logger).Log("msg", "failed to wrap response to png", "err", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Write([]byte(result))
	})
}

func (e *kryptoEcMiddleware) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, err := e.wrapImpl(next, r, w, false)
		if err != nil {
			level.Debug(e.logger).Log("msg", "failed to wrap response", "err", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Write([]byte(result))
	})
}

func (e *kryptoEcMiddleware) wrapImpl(next http.Handler, r *http.Request, w http.ResponseWriter, toPng bool) (string, error) {
	bhr := &bufferedHttpResponse{}
	next.ServeHTTP(bhr, r)

	boxRaw := r.URL.Query().Get("box")
	if boxRaw == "" {
		// level.Debug(e.logger).Log("msg", "no data in box query parameter")
		//	w.WriteHeader(http.StatusUnauthorized)
		return "", fmt.Errorf("no data in box query parameter")
	}

	outerChallenge, err := marshalOuterChallenge(boxRaw)
	if err != nil {
		// level.Debug(e.logger).Log("msg", "failed to marshal outer challenge", "err", err)
		// w.WriteHeader(http.StatusUnauthorized)
		return "", fmt.Errorf("marshaling outer challenge: %w", err)
	}

	var responseBytes []byte
	switch toPng {
	case true:
		responseBytes, err = challenge.RespondPng(e.signer, e.counterParty, *outerChallenge, bhr.Bytes())
		if err != nil {
			// level.Debug(e.logger).Log("msg", "failed to create challenge response to png", "err", err)
			return "", fmt.Errorf("failed to create challenge response to png: %w", err)
		}

	case false:
		outerResponse, err := challenge.Respond(e.signer, e.counterParty, *outerChallenge, bhr.Bytes())
		if err != nil {
			// level.Debug(e.logger).Log("msg", "failed to create challenge response to png", "err", err)
			return "", fmt.Errorf("failed to create challenge response: %w", err)
		}

		responseBytes, err = msgpack.Marshal(outerResponse)
		if err != nil {
			// level.Debug(e.logger).Log("msg", "failed to create challenge response to msgpack", "err", err)
			return "", fmt.Errorf("failed to marshal challenge response to msgpack: %w", err)
		}
	}

	return base64.StdEncoding.EncodeToString(responseBytes), nil
}

func marshalOuterChallenge(boxRaw string) (*challenge.OuterChallenge, error) {
	box, err := base64.StdEncoding.DecodeString(boxRaw)
	if err != nil {
		return nil, fmt.Errorf("unable to base64 decode box: %w", err)
	}

	var outerChallenge challenge.OuterChallenge
	if err := msgpack.Unmarshal(box, &outerChallenge); err != nil {
		return nil, fmt.Errorf("unable to marshall outer challenge: %w", err)
	}
	return &outerChallenge, nil
}
