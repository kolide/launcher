package localserver

import (
	"crypto/ecdsa"
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

	"maps"

	"github.com/kolide/krypto/pkg/echelper"
	"github.com/lestrrat-go/jwx/jwk"
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

	t.Run("handles missing bad b64", func(t *testing.T) {
		t.Parallel()
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/?payload=badb64", nil))
		require.Equal(t, http.StatusBadRequest, rr.Code,
			"should return bad request when box param is not valid b64",
		)
	})

	t.Run("handles chain unmarshall failure", func(t *testing.T) {
		t.Parallel()
		rr := httptest.NewRecorder()
		encoded := base64.StdEncoding.EncodeToString([]byte("badchain"))
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/?payload=%s", encoded), nil))
		require.Equal(t, http.StatusBadRequest, rr.Code,
			"should return bad request when chain cannot be unmarshalled",
		)
	})

	t.Run("handles invalid chain unmarshall failure", func(t *testing.T) {
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

		b64 := base64.StdEncoding.EncodeToString(chainMarshalled)

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

		b64 := base64.StdEncoding.EncodeToString(chainMarshalled)

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/?payload=%s", url.QueryEscape(b64)), nil))
		require.Equal(t, http.StatusOK, rr.Code,
			"should return ok when chain is valid",
		)

		bodyBytes, err := io.ReadAll(rr.Body)
		require.NoError(t, err)

		bodyDecoded, err := base64.StdEncoding.DecodeString(string(bodyBytes))
		require.NoError(t, err)

		var ztaResponse ztaResponse
		require.NoError(t, json.Unmarshal(bodyDecoded, &ztaResponse))

		opened, err := echelper.OpenNaCl(ztaResponse.Data, ztaResponse.PubKey, callerPrivKey)
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
	pubKeyMap := make(map[string]*ecdsa.PublicKey)
	for i := range keyCount {
		keys[i] = mustGenEcdsaKey(t)
		pubKeyMap[fmt.Sprint(i)] = keys[i].Public().(*ecdsa.PublicKey)
	}

	pubEncryptionKey, _, err := box.GenerateKey(rand.Reader)
	require.NoError(t, err)

	t.Run("trusted root is nil", func(t *testing.T) {
		t.Parallel()
		chain, err := newChain(pubEncryptionKey, keys...)
		require.NoError(t, err)
		require.Error(t, chain.validate(nil),
			"should not be able to validate with nil trusted root",
		)
	})

	t.Run("empty chain", func(t *testing.T) {
		t.Parallel()

		chain := &chain{}
		require.Error(t, chain.validate(pubKeyMap),
			"should not be able to validate empty chain",
		)
	})

	t.Run("different trusted root", func(t *testing.T) {
		t.Parallel()

		chain, err := newChain(pubEncryptionKey, keys...)
		require.NoError(t, err)

		badMap := make(map[string]*ecdsa.PublicKey)
		maps.Copy(badMap, pubKeyMap)

		badMap["0"] = mustGenEcdsaKey(t).Public().(*ecdsa.PublicKey)

		require.Error(t, chain.validate(badMap),
			"should not be able to validate chain with different trusted root",
		)
	})

	t.Run("bad sig last item in chain", func(t *testing.T) {
		t.Parallel()

		chain, err := newChain(pubEncryptionKey, keys...)
		require.NoError(t, err)

		// replace last item in chain
		chain.Links[len(chain.Links)-1].Signature = []byte("ahhhh")

		require.Error(t, chain.validate(pubKeyMap),
			"should not be able to validate chain with malformed key",
		)
	})

	t.Run("invalid signature in chain", func(t *testing.T) {
		t.Parallel()

		chain, err := newChain(pubEncryptionKey, keys...)
		require.NoError(t, err)

		chain.Links[1].Payload = "ahhhh"

		require.Error(t, chain.validate(pubKeyMap),
			"should not be able to validate chain with malformed key",
		)
	})

	t.Run("invalid root self sign", func(t *testing.T) {
		t.Parallel()

		chain, err := newChain(pubEncryptionKey, keys...)
		require.NoError(t, err)

		chain.Links[0].Signature = []byte("ahhhh")

		require.Error(t, chain.validate(pubKeyMap),
			"should not be able to validate chain with malformed key",
		)
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

		key, err := jwk.New(badPubEncryptionKey[:])
		require.NoError(t, err)

		thisPayload.PublicKey = key

		payloadBytes, err = json.Marshal(thisPayload)
		require.NoError(t, err)

		chain.Links[len(chain.Links)-1].Payload = base64.URLEncoding.EncodeToString(payloadBytes)

		require.Error(t, chain.validate(pubKeyMap),
			"should not be able to validate chain with malformed key",
		)
	})

	t.Run("valid chain", func(t *testing.T) {
		t.Parallel()

		chain, err := newChain(pubEncryptionKey, keys...)
		require.NoError(t, err)

		require.NoError(t, chain.validate(pubKeyMap),
			"should be able to validate chain",
		)
	})
}

// newChain creates a new chain of keys, where each key signs the next key in the chain.
func newChain(counterPartyPubEncryptionKey *[32]byte, ecdsaKeys ...*ecdsa.PrivateKey) (*chain, error) {
	if len(ecdsaKeys) == 0 {
		return nil, errors.New("no keys provided")
	}

	if len(ecdsaKeys) == 1 {
		return nil, errors.New("only one key provided")
	}

	// data will be last link in links
	links := make([]chainLink, len(ecdsaKeys))

	for i := range len(ecdsaKeys) - 1 {
		parentKey := ecdsaKeys[i]

		childKey := ecdsaKeys[i+1]
		jwkPubKey, err := jwk.New(childKey.Public().(*ecdsa.PublicKey))
		jwkPubKey.Set("kid", fmt.Sprint(i))

		if err != nil {
			return nil, fmt.Errorf("failed to create jwk from ecdsa key: %w", err)
		}

		payload := payload{
			AccountUuid:    "some_account",
			DateTimeSigned: time.Now().Unix(),
			PublicKey:      jwkPubKey,
			UserUuid:       "some_user",
			Ttl:            3600,
			Version:        "1",
		}

		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}

		payloadB64 := base64.URLEncoding.EncodeToString(payloadBytes)

		sig, err := echelper.Sign(parentKey, []byte(payloadB64))
		if err != nil {
			return nil, fmt.Errorf("failed to self sign root key: %w", err)
		}

		links[i] = chainLink{
			Payload:   payloadB64,
			Signature: sig,
			SignedBy:  fmt.Sprint(i),
		}
	}

	lastKey := ecdsaKeys[len(ecdsaKeys)-1]

	pubEncryptionKey, err := jwk.New(counterPartyPubEncryptionKey[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create jwk from x25519 key: %w", err)
	}

	payload := payload{
		AccountUuid:    "some_account",
		DateTimeSigned: time.Now().Unix(),
		PublicKey:      pubEncryptionKey,
		UserUuid:       "some_user",
		Ttl:            3600,
		Version:        "1",
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	payloadB64 := base64.URLEncoding.EncodeToString(payloadBytes)

	// sign data with the last key
	sig, err := echelper.Sign(lastKey, []byte(payloadB64))
	if err != nil {
		return nil, fmt.Errorf("failed to sign data with last key: %w", err)
	}

	links[len(ecdsaKeys)-1] = chainLink{
		Payload:   payloadB64,
		Signature: sig,
		SignedBy:  fmt.Sprint(len(ecdsaKeys) - 1),
	}

	return &chain{Links: links}, nil
}
