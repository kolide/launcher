package localserver

import (
	"crypto"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/krypto/pkg/challenge"
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

		var cmdReq cmdRequestType
		if err := json.Unmarshal(challengeBox.RequestData(), &cmdReq); err != nil {
			level.Debug(e.logger).Log("msg", "unable to unmarshal cmd request", "err", err)
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

		bhr := &bufferedHttpResponse{}
		next.ServeHTTP(bhr, newReq)

		var response []byte

		switch cmdReq.Cmd {
		case "id":
			response, err = challengeBox.Respond(e.signer, bhr.Bytes())
		case "id.png":
			// TODO: png stuff
			response, err = challengeBox.RespondPng(e.signer, bhr.Bytes())
		}

		if err != nil {
			level.Debug(e.logger).Log("msg", "failed to respond", "err", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Write([]byte(base64.StdEncoding.EncodeToString(response)))
	})
}

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
