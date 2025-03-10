package localserver

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/kolide/krypto/pkg/echelper"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/nacl/box"
)

func Test_ZtaAuthMiddleware(t *testing.T) {
	rootTrustedEcKey := mustGenEcdsaKey(t)

	ztaMiddleware := &ztaAuthMiddleware{
		counterPartyKeys: map[string]*ecdsa.PublicKey{
			"for_funzies": mustGenEcdsaKey(t).Public().(*ecdsa.PublicKey),
			"test":        rootTrustedEcKey.Public().(*ecdsa.PublicKey),
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
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/?box=badb64", nil))
		require.Equal(t, http.StatusBadRequest, rr.Code,
			"should return bad request when box param is not valid b64",
		)
	})

	t.Run("handles chain unmarshall failure", func(t *testing.T) {
		t.Parallel()
		rr := httptest.NewRecorder()
		encoded := base64.StdEncoding.EncodeToString([]byte("badchain"))
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/?box=%s", encoded), nil))
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

		chain, err := newChain([]byte("whatevs"), invalidKeys...)
		require.NoError(t, err)

		chainMarshalled, err := json.Marshal(chain)
		require.NoError(t, err)

		b64 := base64.StdEncoding.EncodeToString(chainMarshalled)

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/?box=%s", url.QueryEscape(b64)), nil))
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

		chain, err := newChain(callerPubKey[:], validKeys...)
		require.NoError(t, err)

		chainMarshalled, err := json.Marshal(chain)
		require.NoError(t, err)

		b64 := base64.StdEncoding.EncodeToString(chainMarshalled)

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/?box=%s", url.QueryEscape(b64)), nil))
		require.Equal(t, http.StatusOK, rr.Code,
			"should return ok when chain is valid",
		)

		bodyBytes, err := io.ReadAll(rr.Body)
		require.NoError(t, err)

		bodyDecoded, err := base64.StdEncoding.DecodeString(string(bodyBytes))
		require.NoError(t, err)

		var ztaResponse ztaResponseBox
		require.NoError(t, json.Unmarshal(bodyDecoded, &ztaResponse))

		opened, err := echelper.OpenNaCl(ztaResponse.Data, &ztaResponse.PubKey, callerPrivKey)
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
	for i := range keyCount {
		keys[i] = mustGenEcdsaKey(t)
	}

	t.Run("trusted root is nil", func(t *testing.T) {
		t.Parallel()
		chain, err := newChain([]byte("whatevs"), keys...)
		require.NoError(t, err)
		require.Error(t, chain.validate(nil),
			"should not be able to validate with nil trusted root",
		)
	})

	t.Run("empty chain", func(t *testing.T) {
		t.Parallel()

		chain := &chain{}
		require.Error(t, chain.validate(&keys[0].PublicKey),
			"should not be able to validate empty chain",
		)
	})

	t.Run("different trusted root", func(t *testing.T) {
		t.Parallel()

		chain, err := newChain([]byte("whatevs"), keys...)
		require.NoError(t, err)

		require.Error(t, chain.validate(&keys[1].PublicKey),
			"should not be able to validate chain with different trusted root",
		)
	})

	t.Run("bad key parsing", func(t *testing.T) {
		t.Parallel()

		chain, err := newChain([]byte("whatevs"), keys...)
		require.NoError(t, err)

		chain.Links[0].Data = []byte("badkey")

		require.Error(t, chain.validate(&keys[0].PublicKey),
			"should not be able to validate chain with malformed key",
		)
	})

	t.Run("bad sig last item in chain", func(t *testing.T) {
		t.Parallel()

		chain, err := newChain([]byte("whatevs"), keys...)
		require.NoError(t, err)

		// replace last item in chain
		chain.Links[len(chain.Links)-1].Data = []byte("something_different")

		require.Error(t, chain.validate(&keys[0].PublicKey),
			"should not be able to validate chain with malformed key",
		)
	})

	t.Run("invalid signature in chain", func(t *testing.T) {
		t.Parallel()

		chain, err := newChain([]byte("whatevs"), keys...)
		require.NoError(t, err)

		badKey := mustGenEcdsaKey(t)
		badPubDer, err := x509.MarshalPKIXPublicKey(&badKey.PublicKey)
		require.NoError(t, err)

		chain.Links[1].Data = badPubDer

		require.Error(t, chain.validate(&keys[0].PublicKey),
			"should not be able to validate chain with malformed key",
		)
	})

	t.Run("invalid root self sign", func(t *testing.T) {
		t.Parallel()

		chain, err := newChain([]byte("whatevs"), keys...)
		require.NoError(t, err)

		badKey := mustGenEcdsaKey(t)
		badPubDer, err := x509.MarshalPKIXPublicKey(&badKey.PublicKey)
		require.NoError(t, err)

		badPubDerSig, err := echelper.Sign(badKey, badPubDer)
		require.NoError(t, err)

		chain.Links[0].Sig = badPubDerSig

		require.Error(t, chain.validate(&keys[0].PublicKey),
			"should not be able to validate chain with malformed key",
		)
	})

	t.Run("valid chain", func(t *testing.T) {
		t.Parallel()

		chain, err := newChain([]byte("whatevs"), keys...)
		require.NoError(t, err)

		require.NoError(t, chain.validate(&keys[0].PublicKey),
			"should be able to validate chain",
		)
	})
}

// newChain creates a new chain of keys, where each key signs the next key in the chain.
func newChain(data []byte, ecdsaKeys ...*ecdsa.PrivateKey) (*chain, error) {
	if len(ecdsaKeys) == 0 {
		return nil, errors.New("no keys provided")
	}

	if len(ecdsaKeys) == 1 {
		return nil, errors.New("only one key provided")
	}

	// data will be last link in links
	links := make([]chainLink, len(ecdsaKeys)+1)

	rootDer, err := x509.MarshalPKIXPublicKey(&ecdsaKeys[0].PublicKey)
	if err != nil {
		return nil, err
	}

	sig, err := echelper.Sign(ecdsaKeys[0], rootDer)
	if err != nil {
		return nil, fmt.Errorf("failed to self sign root key: %w", err)
	}

	// The first link is self-signed.
	links[0] = chainLink{
		Data: rootDer,
		Sig:  sig,
	}

	for i := range len(ecdsaKeys) - 1 {
		parentKey := ecdsaKeys[i]

		childKey := ecdsaKeys[i+1]

		childDer, err := x509.MarshalPKIXPublicKey(&childKey.PublicKey)
		if err != nil {
			return nil, err
		}

		sig, err := echelper.Sign(parentKey, childDer)
		if err != nil {
			return nil, fmt.Errorf("failed to sign DER: %w", err)
		}

		links[i+1] = chainLink{
			Data: childDer,
			Sig:  sig,
		}
	}

	lastKey := ecdsaKeys[len(ecdsaKeys)-1]

	// sign data with the last key
	sig, err = echelper.Sign(lastKey, data)
	if err != nil {
		return nil, fmt.Errorf("failed to sign data with last key: %w", err)
	}

	links[len(ecdsaKeys)] = chainLink{
		Data: data,
		Sig:  sig,
	}

	return &chain{Links: links}, nil
}
