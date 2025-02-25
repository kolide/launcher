package localserver

import (
	"crypto/ecdsa"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/kolide/krypto/pkg/echelper"
	"github.com/vmihailenco/msgpack/v5"
)

type ztaResponseBox struct {
	Data   []byte   `msgpack:"data"`
	PubKey [32]byte `msgpack:"pubKey"`
}

type ztaAuthMiddleware struct {
	counterPartyPubKey *ecdsa.PublicKey
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

		if err := chain.validate(z.counterPartyPubKey); err != nil {
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

		data, err := msgpack.Marshal(ztaResponse)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write([]byte(base64.StdEncoding.EncodeToString(data)))
	})
}

type chainLink struct {
	Data []byte `msgpack:"data"`
	Sig  []byte `msgpack:"sig"`
}

type chain struct {
	Links []chainLink `msgpack:"links"`
}

func unmarshallChain(data []byte) (*chain, error) {
	var c chain
	if err := msgpack.Unmarshal(data, &c); err != nil {
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

	for i := 0; i < len(c.Links)-1; i++ {
		parentKey, err := echelper.PublicB64DerToEcdsaKey(c.Links[i].Data)
		if err != nil {
			return fmt.Errorf("failed to convert public key from DER: %w", err)
		}

		// The first key in the chain must match the trusted root
		if i == 0 && !equalPubEcdsaKeys(trustedRoot, parentKey) {
			return errors.New("first key in chain does not match trusted root")
		}

		if err := echelper.VerifySignature(parentKey, c.Links[i+1].Data, c.Links[i+1].Sig); err != nil {
			return fmt.Errorf("failed to verify signature: %w", err)
		}
	}

	return nil
}

// EqualPublicKeys compares two ECDSA public keys for equality.
func equalPubEcdsaKeys(k1, k2 *ecdsa.PublicKey) bool {
	// Both nil => equal
	if k1 == nil && k2 == nil {
		return true
	}
	// One nil => not equal
	if k1 == nil || k2 == nil {
		return false
	}
	// Compare curve (assuming standard library named curves)
	if k1.Curve != k2.Curve {
		return false
	}
	// Compare big.Int values for X, Y
	if k1.X.Cmp(k2.X) != 0 {
		return false
	}
	if k1.Y.Cmp(k2.Y) != 0 {
		return false
	}
	return true
}
