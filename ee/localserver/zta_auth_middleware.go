package localserver

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/kolide/krypto/pkg/echelper"
)

type ztaResponseBox struct {
	Data   []byte   `json:"data"`
	PubKey [32]byte `json:"pubKey"`
}

type ztaAuthMiddleware struct {
	counterPartyKeys map[string]*ecdsa.PublicKey
}

func (z *ztaAuthMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		boxParam := r.URL.Query().Get("box")
		if boxParam == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		boxBytes, err := base64.StdEncoding.DecodeString(boxParam)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		chain, err := unmarshallChain(boxBytes)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		chainValidated := false

		for _, key := range z.counterPartyKeys {
			if err := chain.validate(key); err != nil {
				continue
			}
			chainValidated = true
			break
		}

		if !chainValidated {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		newReq := &http.Request{
			Method: http.MethodGet,
			Header: make(http.Header),
			URL: &url.URL{
				Scheme: r.URL.Scheme,
				Host:   r.Host,
				Path:   r.URL.Path,
			},
		}

		bhr := &bufferedHttpResponse{}
		next.ServeHTTP(bhr, newReq)

		// expect the last link in the chain to be the public encryption key
		counterPartyPublicEncryptionKeyBytes := chain.Links[len(chain.Links)-1].Data
		counterPartyPublicEncryptionKey := new([32]byte)
		copy(counterPartyPublicEncryptionKey[:], counterPartyPublicEncryptionKeyBytes)

		box, pubKey, err := echelper.SealNaCl(bhr.Bytes(), counterPartyPublicEncryptionKey)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		ztaResponse := ztaResponseBox{
			Data:   box,
			PubKey: *pubKey,
		}

		data, err := json.Marshal(ztaResponse)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write([]byte(base64.StdEncoding.EncodeToString(data)))
	})
}

type chainLink struct {
	Data []byte `json:"data"`
	Sig  []byte `json:"sig"`
}

type chain struct {
	Links []chainLink `json:"links"`
}

func unmarshallChain(data []byte) (*chain, error) {
	var c chain
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal chain: %w", err)
	}
	return &c, nil
}

func (c *chain) validate(trustedRoot *ecdsa.PublicKey) error {
	if trustedRoot == nil {
		return errors.New("trusted root is nil")
	}

	if len(c.Links) == 0 {
		return errors.New("chain is empty")
	}

	for i := range len(c.Links) {
		if c.Links[i].Data == nil || c.Links[i].Sig == nil {
			return fmt.Errorf("link %d is missing data or sig", i)
		}
	}

	for i := range len(c.Links) - 1 {
		parentKey, err := x509.ParsePKIXPublicKey(c.Links[i].Data)
		if err != nil {
			return fmt.Errorf("failed to convert public key from DER: %w", err)
		}

		// verify it's an ecdsa key
		parentEcdsa, ok := parentKey.(*ecdsa.PublicKey)
		if !ok {
			return errors.New("parent key is not an ECDSA key")
		}

		// The first key in the chain must match the trusted root and be self signed
		if i == 0 {
			if !trustedRoot.Equal(parentEcdsa) {
				return errors.New("first key in chain does not match trusted root")
			}

			if err := echelper.VerifySignature(trustedRoot, c.Links[i].Data, c.Links[i].Sig); err != nil {
				return fmt.Errorf("failed to verify root self signature: %w", err)
			}
		}

		if err := echelper.VerifySignature(parentEcdsa, c.Links[i+1].Data, c.Links[i+1].Sig); err != nil {
			return fmt.Errorf("failed to verify signature: %w", err)
		}
	}

	return nil
}
