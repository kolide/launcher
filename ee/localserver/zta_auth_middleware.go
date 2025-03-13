package localserver

import (
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/kolide/krypto/pkg/echelper"
	"github.com/lestrrat-go/jwx/jwk"
)

type ztaAuthMiddleware struct {
	counterPartyKeys map[string]*ecdsa.PublicKey
}

type ztaResponse struct {
	Data   []byte    `json:"data"`
	PubKey *[32]byte `json:"pubKey"`
}

func (z *ztaAuthMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		boxParam := r.URL.Query().Get("payload")
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

		if err := chain.validate(z.counterPartyKeys); err != nil {
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

		box, pubKey, err := echelper.SealNaCl(bhr.Bytes(), chain.counterPartyPubEncryptionKey)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		ztaResponse := ztaResponse{
			Data:   box,
			PubKey: pubKey,
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

type payload struct {
	AccountUuid    string  `json:"accountUuid"`
	DateTimeSigned int64   `json:"dateTimeSigned"`
	PublicKey      jwk.Key `json:"publicKey"`
	UserUuid       string  `json:"userUuid"`
	Ttl            int64   `json:"ttl"`
	Version        string  `json:"version"`
}

func (p *payload) UnmarshalJSON(data []byte) error {
	// Create a temporary struct with PublicKey as json.RawMessage
	type tmpPayload struct {
		AccountUuid    string          `json:"accountUuid"`
		DateTimeSigned int64           `json:"dateTimeSigned"`
		PublicKey      json.RawMessage `json:"publicKey"`
		UserUuid       string          `json:"userUuid"`
		Ttl            int64           `json:"ttl"`
		Version        string          `json:"version"`
	}

	var tmp tmpPayload
	if err := json.Unmarshal(data, &tmp); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	// Parse the publicKey using jwk.ParseKey
	key, err := jwk.ParseKey(tmp.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to parse publicKey: %w", err)
	}

	// Populate the payload struct
	p.AccountUuid = tmp.AccountUuid
	p.DateTimeSigned = tmp.DateTimeSigned
	p.PublicKey = key
	p.UserUuid = tmp.UserUuid
	p.Ttl = tmp.Ttl
	p.Version = tmp.Version

	return nil
}

type chainLink struct {
	Payload   string `json:"payload"`
	Signature []byte `json:"signature"`
	SignedBy  string `json:"signedBy"`
}

type chain struct {
	Links                        []chainLink `json:"links"`
	counterPartyPubEncryptionKey *[32]byte
}

func unmarshallChain(data []byte) (*chain, error) {
	var c chain
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal chain: %w", err)
	}
	return &c, nil
}

func (c *chain) validate(trustedKeys map[string]*ecdsa.PublicKey) error {
	if len(c.Links) == 0 {
		return errors.New("chain is empty")
	}

	for i := range len(c.Links) {
		if c.Links[i].Payload == "" || c.Links[i].Signature == nil {
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

	if !ok {
		return fmt.Errorf("root key with kid %s not found in trusted keys", c.Links[0].SignedBy)
	}

	for i := range len(c.Links) - 1 {
		if err := echelper.VerifySignature(parentEcdsa, []byte(c.Links[i].Payload), c.Links[i].Signature); err != nil {
			return fmt.Errorf("failed to verify signature: %w", err)
		}

		payloadBytes, err := base64.URLEncoding.DecodeString(c.Links[i].Payload)
		if err != nil {
			return fmt.Errorf("failed to decode payload: %w", err)
		}

		var thisPayload payload
		if err := json.Unmarshal(payloadBytes, &thisPayload); err != nil {
			return fmt.Errorf("failed to unmarshal payload: %w", err)
		}

		if err := thisPayload.PublicKey.Raw(parentEcdsa); err != nil {
			return fmt.Errorf("failed to extract public key from payload: %w", err)
		}
	}

	// use the last parent key to verify the last link
	if err := echelper.VerifySignature(parentEcdsa, []byte(c.Links[len(c.Links)-1].Payload), c.Links[len(c.Links)-1].Signature); err != nil {
		return fmt.Errorf("failed to verify last link signature: %w", err)
	}

	// unmarshal the last payload
	payloadBytes, err := base64.URLEncoding.DecodeString(c.Links[len(c.Links)-1].Payload)
	if err != nil {
		return fmt.Errorf("failed to decode payload: %w", err)
	}

	var lastPayload payload
	if err := json.Unmarshal(payloadBytes, &lastPayload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	var raw any
	if err := lastPayload.PublicKey.Raw(&raw); err != nil {
		return fmt.Errorf("failed to extract last public key from payload: %w", err)
	}

	// Depending on the library version, raw might be a []byte or a *[32]byte.
	var pubKeyBytes []byte
	switch v := raw.(type) {
	case []byte:
		pubKeyBytes = v
	case *[32]byte:
		pubKeyBytes = v[:]
	default:
		return fmt.Errorf("unexpected type for raw: %T", raw)
	}

	if len(pubKeyBytes) != 32 {
		return errors.New("invalid public key length")
	}

	c.counterPartyPubEncryptionKey = new([32]byte)
	copy(c.counterPartyPubEncryptionKey[:], pubKeyBytes)

	return nil
}
