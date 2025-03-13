package localserver

import (
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

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

		boxBytes, err := base64.URLEncoding.DecodeString(boxParam)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var myChain chain
		if err := json.Unmarshal(boxBytes, &myChain); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if err := myChain.validate(z.counterPartyKeys); err != nil {
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

		box, pubKey, err := echelper.SealNaCl(bhr.Bytes(), myChain.counterPartyPubEncryptionKey)
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
		w.Write([]byte(base64.URLEncoding.EncodeToString(data)))
	})
}

type payload struct {
	AccountUuid    string  `json:"accountUuid"`
	DateTimeSigned int64   `json:"dateTimeSigned"`
	Environment    string  `json:"environment"`
	ExpirationDate int64   `json:"expirationDate"`
	PublicKey      jwk.Key `json:"publicKey"`
	SignedBy       string  `json:"signedBy"`
	UserUuid       string  `json:"userUuid"`
	Version        uint8   `json:"version"`
}

func (p *payload) UnmarshalJSON(data []byte) error {
	// Create a temporary struct with PublicKey as json.RawMessage
	type tmpPayload struct {
		AccountUuid    string          `json:"accountUuid"`
		DateTimeSigned int64           `json:"dateTimeSigned"`
		Environment    string          `json:"environment"`
		ExpirationDate int64           `json:"expirationDate"`
		PublicKey      json.RawMessage `json:"publicKey"`
		SignedBy       string          `json:"signedBy"`
		UserUuid       string          `json:"userUuid"`
		Version        uint8           `json:"version"`
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

	p.AccountUuid = tmp.AccountUuid
	p.DateTimeSigned = tmp.DateTimeSigned
	p.Environment = tmp.Environment
	p.ExpirationDate = tmp.ExpirationDate
	p.PublicKey = key
	p.SignedBy = tmp.SignedBy
	p.UserUuid = tmp.UserUuid
	p.Version = tmp.Version

	return nil
}

type chainLink struct {
	Payload   string `json:"payload"`
	Signature []byte `json:"signature"`
	SignedBy  string `json:"signedBy"`
}

type chain struct {
	Links []chainLink `json:"links"`
	// counterPartyPubEncryptionKey is the last public key in the chain, which is a x25519 key
	// we set this after we extract and verify in the validate method
	counterPartyPubEncryptionKey *[32]byte
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

func (c *chain) MarshalJSON() ([]byte, error) {
	// outgoing data is just an aray of chain links
	bytes, err := json.Marshal(c.Links)

	if err != nil {
		return nil, fmt.Errorf("failed to marshal chain links: %w", err)
	}
	return bytes, nil
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

	var lastPayload payload

	for i := range len(c.Links) {
		if err := echelper.VerifySignature(parentEcdsa, []byte(c.Links[i].Payload), c.Links[i].Signature); err != nil {
			return fmt.Errorf("invalid signature: %w", err)
		}

		payloadBytes, err := base64.URLEncoding.DecodeString(c.Links[i].Payload)
		if err != nil {
			return fmt.Errorf("failed to decode payload: %w", err)
		}

		if err := json.Unmarshal(payloadBytes, &lastPayload); err != nil {
			return fmt.Errorf("failed to unmarshal payload: %w", err)
		}

		if lastPayload.ExpirationDate < time.Now().Unix() {
			return fmt.Errorf("payload %d has expired", i)
		}

		if i < len(c.Links)-1 {
			// last key is not p256 ecdsa so don't reassign it
			// we handle last key after the loop
			if err := lastPayload.PublicKey.Raw(parentEcdsa); err != nil {
				return fmt.Errorf("failed to extract public key from payload: %w", err)
			}
		}
	}

	// now we have verified all the entire chain, and we have the last
	// public key, which is a x25519 key, extract and set as the counter party key
	var rawX25519Key any
	if err := lastPayload.PublicKey.Raw(&rawX25519Key); err != nil {
		return fmt.Errorf("failed to extract last public key from payload: %w", err)
	}

	// Depending on the library version, raw might be a []byte or a *[32]byte.
	var pubKeyBytes []byte
	switch v := rawX25519Key.(type) {
	case []byte:
		pubKeyBytes = v
	case *[32]byte:
		pubKeyBytes = v[:]
	default:
		return fmt.Errorf("unexpected type for last public key: %T", rawX25519Key)
	}

	if len(pubKeyBytes) != 32 {
		return errors.New("invalid public key length")
	}

	// set the counter party key
	c.counterPartyPubEncryptionKey = new([32]byte)
	copy(c.counterPartyPubEncryptionKey[:], pubKeyBytes)

	return nil
}
