package localserver

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/nacl/box"
)

func Test_ZtaAuthMiddleware(t *testing.T) {
	rootTrustedEcKey := mustGenEcdsaKey(t)

	ztaMiddleware := &ztaAuthMiddleware{
		counterPartyKeys: map[string]*ecdsa.PublicKey{
			"for_funzies": mustGenEcdsaKey(t).Public().(*ecdsa.PublicKey),
			"0":           rootTrustedEcKey.Public().(*ecdsa.PublicKey), // this is the trusted root
		},
		slogger: multislogger.NewNopLogger(),
	}

	returnData := []byte("Congrats!, you got the data back!")

	handler := ztaMiddleware.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(returnData)
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("handles missing box param", func(t *testing.T) {
		t.Parallel()
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
		require.Equal(t, http.StatusBadRequest, rr.Code,
			"should return bad request when box param is missing",
		)
	})

	t.Run("handles bad b64", func(t *testing.T) {
		t.Parallel()
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/?payload=badb64", nil))
		require.Equal(t, http.StatusBadRequest, rr.Code,
			"should return bad request when box param is not valid b64",
		)
	})

	t.Run("handles empty b64", func(t *testing.T) {
		t.Parallel()
		rr := httptest.NewRecorder()
		payload := base64.URLEncoding.EncodeToString([]byte("[]"))
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/?payload=%s", payload), http.NoBody))
		require.Equal(t, http.StatusUnauthorized, rr.Code,
			"should return bad request when box param is valid b64 but empty",
		)
	})

	t.Run("handles chain unmarshall failure", func(t *testing.T) {
		t.Parallel()
		rr := httptest.NewRecorder()
		encoded := base64.URLEncoding.EncodeToString([]byte("badchain"))
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/?payload=%s", encoded), nil))
		require.Equal(t, http.StatusBadRequest, rr.Code,
			"should return bad request when chain cannot be unmarshalled",
		)
	})

	t.Run("handles invalid chain", func(t *testing.T) {
		t.Parallel()

		invalidKeys := make([]*ecdsa.PrivateKey, 3)
		for i := range invalidKeys {
			invalidKeys[i] = mustGenEcdsaKey(t)
		}

		pubcallerPubKey, _, err := box.GenerateKey(rand.Reader)
		require.NoError(t, err)

		chain, err := newChain(pubcallerPubKey, invalidKeys...)
		require.NoError(t, err)

		chainMarshalled, err := json.Marshal(chain)
		require.NoError(t, err)

		b64 := base64.URLEncoding.EncodeToString(chainMarshalled)

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/?payload=%s", url.QueryEscape(b64)), nil))
		require.Equal(t, http.StatusUnauthorized, rr.Code,
			"should return unauthorized when chain cannot be validated",
		)
	})

	t.Run("handles valid chain", func(t *testing.T) {
		t.Parallel()

		validKeys := make([]*ecdsa.PrivateKey, 4)
		validKeys[0] = rootTrustedEcKey

		for i := 1; i < len(validKeys); i++ {
			validKeys[i] = mustGenEcdsaKey(t)
		}

		callerPubKey, callerPrivKey, err := box.GenerateKey(rand.Reader)
		require.NoError(t, err)

		chain, err := newChain(callerPubKey, validKeys...)
		require.NoError(t, err)

		chainMarshalled, err := json.Marshal(chain)
		require.NoError(t, err)

		b64 := base64.URLEncoding.EncodeToString(chainMarshalled)

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/?payload=%s", b64), nil))
		require.Equal(t, http.StatusOK, rr.Code,
			"should return ok when chain is valid",
		)

		bodyBytes, err := io.ReadAll(rr.Body)
		require.NoError(t, err)

		bodyDecoded, err := base64.StdEncoding.DecodeString(string(bodyBytes))
		require.NoError(t, err)

		var z ztaResponse
		require.NoError(t, json.Unmarshal(bodyDecoded, &z))

		opened, err := echelper.OpenNaCl(z.Data, z.PubKey, callerPrivKey)
		require.NoError(t, err)

		require.Equal(t, returnData, opened,
			"should be able to open NaCl box and get data",
		)
	})
}

func Test_ValidateCertChain(t *testing.T) {
	t.Parallel()

	keyCount := 3
	keys := make([]*ecdsa.PrivateKey, keyCount)
	trustedKeysMap := make(map[string]*ecdsa.PublicKey)
	for i := range keyCount {
		keys[i] = mustGenEcdsaKey(t)
		trustedKeysMap[fmt.Sprint(i)] = keys[i].Public().(*ecdsa.PublicKey)
	}

	pubEncryptionKey, _, err := box.GenerateKey(rand.Reader)
	require.NoError(t, err)

	t.Run("trusted root is nil", func(t *testing.T) {
		t.Parallel()
		chain, err := newChain(pubEncryptionKey, keys...)
		require.NoError(t, err)

		chain.Links[0] = chainLink{}

		require.ErrorContains(t, chain.validate(trustedKeysMap), "missing data")
	})

	t.Run("empty chain", func(t *testing.T) {
		t.Parallel()

		chain := &chain{}
		require.ErrorContains(t, chain.validate(trustedKeysMap), "chain is empty")
	})

	t.Run("key not found", func(t *testing.T) {
		t.Parallel()

		chain, err := newChain(pubEncryptionKey, keys...)
		require.NoError(t, err)

		require.ErrorContains(t, chain.validate(make(map[string]*ecdsa.PublicKey)), "not found in trusted keys")
	})

	t.Run("bad sig last item in chain", func(t *testing.T) {
		t.Parallel()

		chain, err := newChain(pubEncryptionKey, keys...)
		require.NoError(t, err)

		// replace last item in chain
		chain.Links[len(chain.Links)-1].Signature = base64.URLEncoding.EncodeToString([]byte("bad sig"))

		require.ErrorContains(t, chain.validate(trustedKeysMap), "invalid signature")
	})

	t.Run("invalid signature in chain", func(t *testing.T) {
		t.Parallel()

		chain, err := newChain(pubEncryptionKey, keys...)
		require.NoError(t, err)

		chain.Links[1].Payload = base64.URLEncoding.EncodeToString([]byte("ahhh"))

		require.ErrorContains(t, chain.validate(trustedKeysMap), "invalid signature")
	})

	t.Run("invalid public encryption key", func(t *testing.T) {
		t.Parallel()

		chain, err := newChain(pubEncryptionKey, keys...)
		require.NoError(t, err)

		badPubEncryptionKey, _, err := box.GenerateKey(rand.Reader)
		require.NoError(t, err)

		// unmarshall the last payload
		payloadBytes, err := base64.URLEncoding.DecodeString(chain.Links[len(chain.Links)-1].Payload)
		require.NoError(t, err)

		var thisPayload payload
		require.NoError(t, json.Unmarshal(payloadBytes, &thisPayload))

		thisPayload.PublicKey = jwk{
			Type:  "OKP",
			Curve: "X25519",
			X:     base64.RawURLEncoding.EncodeToString(badPubEncryptionKey[:]),
			KeyID: "bad",
		}

		payloadBytes, err = json.Marshal(thisPayload)
		require.NoError(t, err)

		chain.Links[len(chain.Links)-1].Payload = base64.URLEncoding.EncodeToString(payloadBytes)

		require.ErrorContains(t, chain.validate(trustedKeysMap), "invalid signature")
	})

	t.Run("handles expired chain", func(t *testing.T) {
		t.Parallel()

		chain, err := newChain(pubEncryptionKey, keys...)
		require.NoError(t, err)

		// get first payload
		payloadBytes, err := base64.URLEncoding.DecodeString(chain.Links[0].Payload)
		require.NoError(t, err)

		var thisPayload payload
		require.NoError(t, json.Unmarshal(payloadBytes, &thisPayload))
		thisPayload.ExpirationDate = time.Now().Add(-1 * time.Hour).Unix()

		payloadBytes, err = json.Marshal(thisPayload)
		require.NoError(t, err)

		payloadBytesB64 := base64.URLEncoding.EncodeToString(payloadBytes)

		// sign the bytes with the first key
		sig, err := echelper.Sign(keys[0], []byte(payloadBytesB64))
		require.NoError(t, err)

		chain.Links[0].Payload = payloadBytesB64
		chain.Links[0].Signature = base64.URLEncoding.EncodeToString(sig)

		require.ErrorContains(t, chain.validate(trustedKeysMap), "expired")
	})

	t.Run("valid chain", func(t *testing.T) {
		t.Parallel()

		chain, err := newChain(pubEncryptionKey, keys...)
		require.NoError(t, err)

		require.NoError(t, chain.validate(trustedKeysMap),
			"should be able to validate chain",
		)

		require.Equal(t, chain.counterPartyPubEncryptionKey, pubEncryptionKey,
			"counter party pub encryption key should get set in validate",
		)
	})
}

// newChain creates a new chain of keys, where each key signs the next key in the chain
// leaving this in the _test file since we will always be receiving a chain of keys
// so this is only needed for testing
func newChain(counterPartyPubEncryptionKey *[32]byte, ecdsaKeys ...*ecdsa.PrivateKey) (*chain, error) {
	if len(ecdsaKeys) == 0 {
		return nil, errors.New("no keys provided")
	}

	if len(ecdsaKeys) == 1 {
		return nil, errors.New("only one key provided")
	}

	// counterPartyPubEncryptionKey will be last link in links
	links := make([]chainLink, len(ecdsaKeys))

	for i := range len(ecdsaKeys) {

		parentKey := ecdsaKeys[i]

		var childKey *jwk
		var err error

		if i == len(ecdsaKeys)-1 {
			childKey, err = toJWK(counterPartyPubEncryptionKey, "counter_party_pub_encryption_key")
		} else {
			childKey, err = toJWK(&ecdsaKeys[i+1].PublicKey, fmt.Sprint(i))
		}

		if err != nil {
			return nil, fmt.Errorf("failed to create jwk from public key: %w", err)
		}

		thisPayload := payload{
			AccountUuid:    "some_account",
			DateTimeSigned: time.Now().Unix(),
			PublicKey:      *childKey,
			UserUuid:       "some_user",
			ExpirationDate: time.Now().Add(1 * time.Hour).Unix(),
			Version:        1,
		}

		payloadBytes, err := json.Marshal(thisPayload)
		if err != nil {
			return nil, err
		}

		payloadB64 := base64.URLEncoding.EncodeToString(payloadBytes)

		sig, err := echelper.Sign(parentKey, []byte(payloadB64))
		if err != nil {
			return nil, err
		}

		links[i] = chainLink{
			Payload:   payloadB64,
			Signature: base64.URLEncoding.EncodeToString(sig),
			SignedBy:  fmt.Sprint(i),
		}
	}

	return &chain{Links: links}, nil
}

// toJWK accepts either an *ecdsa.PublicKey or *[32]byte (for X25519)
// along with an optional kid value and returns a jwk object
func toJWK(key any, kid string) (*jwk, error) {
	switch k := key.(type) {
	case *ecdsa.PublicKey:
		// determine curve
		var crv string
		switch k.Curve {
		case elliptic.P256():
			crv = "P-256"
		case elliptic.P384():
			crv = "P-384"
		case elliptic.P521():
			crv = "P-521"
		default:
			return nil, fmt.Errorf("unsupported elliptic curve, %s", crv)
		}

		// Encode x and y coordinates using base64 URL encoding (unpadded).
		xStr := base64.RawURLEncoding.EncodeToString(k.X.Bytes())
		yStr := base64.RawURLEncoding.EncodeToString(k.Y.Bytes())

		return &jwk{
			Type:  "EC",
			Curve: crv,
			X:     xStr,
			Y:     yStr,
			KeyID: kid,
		}, nil

	case *[32]byte: // Handle X25519 public key.
		xStr := base64.RawURLEncoding.EncodeToString(k[:])

		return &jwk{
			Type:  "OKP",
			Curve: "X25519",
			X:     xStr,
		}, nil

	default:
		return nil, errors.New("unsupported key type")
	}
}
