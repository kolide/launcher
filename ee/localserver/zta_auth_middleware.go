package localserver

import (
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"time"

	"github.com/kolide/krypto/pkg/echelper"
)

type ztaAuthMiddleware struct {
	// counterPartyKeys is a map of trusted keys, maps KID to pubkey
	counterPartyKeys map[string]*ecdsa.PublicKey
	slogger          *slog.Logger
}

type ztaResponse struct {
	Data   []byte    `json:"data"`
	PubKey *[32]byte `json:"pubKey"`
}

func (z *ztaAuthMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		boxParam := r.URL.Query().Get("payload")
		if boxParam == "" {
			z.slogger.Log(r.Context(), slog.LevelWarn,
				"missing payload url param",
			)

			w.WriteHeader(http.StatusBadRequest)
			return
		}

		boxBytes, err := base64.URLEncoding.DecodeString(boxParam)
		if err != nil {
			z.slogger.Log(r.Context(), slog.LevelWarn,
				"failed to decode payload",
				"err", err,
			)

			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var requestTrustChain chain
		if err := json.Unmarshal(boxBytes, &requestTrustChain); err != nil {
			z.slogger.Log(r.Context(), slog.LevelWarn,
				"failed to unmarshal chain",
				"err", err,
			)

			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if err := requestTrustChain.validate(z.counterPartyKeys); err != nil {
			z.slogger.Log(r.Context(), slog.LevelWarn,
				"failed to validate chain",
				"err", err,
			)

			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		bhrHeaders := make(http.Header)
		maps.Copy(bhrHeaders, r.Header)

		newReq := &http.Request{
			Method: r.Method,
			Header: bhrHeaders,
			URL: &url.URL{
				Scheme: r.URL.Scheme,
				Host:   r.Host,
				Path:   r.URL.Path,
			},
		}

		bhr := &bufferedHttpResponse{}
		next.ServeHTTP(bhr, newReq)

		box, pubKey, err := echelper.SealNaCl(bhr.Bytes(), requestTrustChain.counterPartyPubEncryptionKey)
		if err != nil {
			z.slogger.Log(r.Context(), slog.LevelError,
				"failed to seal response",
				"err", err,
			)

			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		ztaResponse := ztaResponse{
			Data:   box,
			PubKey: pubKey,
		}

		data, err := json.Marshal(ztaResponse)
		if err != nil {
			z.slogger.Log(r.Context(), slog.LevelError,
				"failed to marshal zta response",
				"err", err,
			)

			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write([]byte(base64.URLEncoding.EncodeToString(data)))
	})
}

type payload struct {
	AccountUuid    string `json:"accountUuid"`
	DateTimeSigned int64  `json:"dateTimeSigned"`
	Environment    string `json:"environment"`
	ExpirationDate int64  `json:"expirationDate"`
	PublicKey      jwk    `json:"publicKey"`
	SignedBy       string `json:"signedBy"`
	UserUuid       string `json:"userUuid"`
	Version        uint8  `json:"version"`
}

// chain represents a chain of trust, where each link in the chain has a payload signed by the key in the previous link.
// The first link in the chain is signed by a trusted root key.
// There can be any number of keys between the root and the last key.
// The last key is an x25519 key which is the public half of a NaCl box key.
type chain struct {
	Links []chainLink `json:"links"`
	// counterPartyPubEncryptionKey is the last public key in the chain, which is a x25519 key
	// we set this after we extract and verify in the validate method
	counterPartyPubEncryptionKey *[32]byte
}

// chainLink represents a link in a chain of trust
type chainLink struct {
	// Payload a b64 url encoded json
	Payload string `json:"payload"`
	// Signature is b64 url encode of signature
	Signature string `json:"signature"`
	// Signed by is the key id "kid" of the key that signed this link
	SignedBy string `json:"signedBy"`
}

func (c *chain) UnmarshalJSON(data []byte) error {
	// incomming data is just an array of chain links
	var links []chainLink
	if err := json.Unmarshal(data, &links); err != nil {
		return fmt.Errorf("failed to unmarshal chain links: %w", err)
	}

	c.Links = links
	return nil
}

// MarshalJSON marshals the chain as a json array of chain links
func (c *chain) MarshalJSON() ([]byte, error) {
	// outgoing data is just an aray of chain links
	bytes, err := json.Marshal(c.Links)

	if err != nil {
		return nil, fmt.Errorf("failed to marshal chain links: %w", err)
	}
	return bytes, nil
}

// validate iterates over all the links in the chain and verifies the signatures and timestamps
// the root p256 ecdsa key must be in the trusted keys map, the map key is the "kid" of the key
func (c *chain) validate(trustedKeys map[string]*ecdsa.PublicKey) error {
	if len(c.Links) == 0 {
		return errors.New("chain is empty")
	}

	for i := range len(c.Links) {
		if c.Links[i].Payload == "" || c.Links[i].Signature == "" {
			return fmt.Errorf("link %d is missing data or sig", i)
		}
	}

	rootKey, ok := trustedKeys[c.Links[0].SignedBy]
	if !ok {
		return fmt.Errorf("root key with kid %s not found in trusted keys", c.Links[0].SignedBy)
	}

	// make a copy of the root key so that we can reassign it
	parentEcdsa := &ecdsa.PublicKey{
		Curve: rootKey.Curve,
		X:     rootKey.X,
		Y:     rootKey.Y,
	}

	var currentPayload payload

	for i := range len(c.Links) {

		signature, err := base64.URLEncoding.DecodeString(c.Links[i].Signature)
		if err != nil {
			return fmt.Errorf("failed to decode signature at index %d: %w", i, err)
		}

		if err := echelper.VerifySignature(parentEcdsa, []byte(c.Links[i].Payload), signature); err != nil {
			return fmt.Errorf("invalid signature at index %d: %w", i, err)
		}

		payloadBytes, err := base64.URLEncoding.DecodeString(c.Links[i].Payload)
		if err != nil {
			return fmt.Errorf("failed to decode payload of index %d: %w", i, err)
		}

		if err := json.Unmarshal(payloadBytes, &currentPayload); err != nil {
			return fmt.Errorf("failed to unmarshal payload of index %d: %w", i, err)
		}

		if currentPayload.ExpirationDate < time.Now().Unix() {
			return fmt.Errorf("payload at index %d has expired, kid %s", i, currentPayload.PublicKey.KeyID)
		}

		if i < len(c.Links)-1 {
			ecdsaPubKey, err := currentPayload.PublicKey.ecdsaPubKey()
			if err != nil {
				return fmt.Errorf("failed to convert public key at index %d, kid %s into public ecdsa key: %w", i, currentPayload.PublicKey.KeyID, err)
			}

			parentEcdsa = ecdsaPubKey
		}
	}

	rawX25519Key, err := currentPayload.PublicKey.x25519PubKey()
	if err != nil {
		return fmt.Errorf("failed to convert last public key into x25519 key: %w", err)
	}

	// set the counter party key
	c.counterPartyPubEncryptionKey = rawX25519Key

	return nil
}
