package localserver

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/kolide/krypto/pkg/echelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"golang.org/x/crypto/nacl/box"
)

func Test_ZtaAuthMiddleware(t *testing.T) {
	rootTrustedEcKey := mustGenEcdsaKey(t)

	ztaMiddleware := &ztaAuthMiddleware{
		counterPartyPubKey: &rootTrustedEcKey.PublicKey,
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
		for i := 0; i < len(invalidKeys); i++ {
			invalidKeys[i] = mustGenEcdsaKey(t)
		}

		chain, err := newChain([]byte("whatevs"), invalidKeys...)
		require.NoError(t, err)

		chainMarshalled, err := msgpack.Marshal(chain)
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

		chainMarshalled, err := msgpack.Marshal(chain)
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
		require.NoError(t, msgpack.Unmarshal(bodyDecoded, &ztaResponse))

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
	for i := 0; i < keyCount; i++ {
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

	t.Run("invalid signature in chain", func(t *testing.T) {
		t.Parallel()

		chain, err := newChain([]byte("whatevs"), keys...)
		require.NoError(t, err)

		badKey := mustGenEcdsaKey(t)
		badPubDer, err := echelper.PublicEcdsaToB64Der(&badKey.PublicKey)
		require.NoError(t, err)

		chain.Links[1].Data = badPubDer

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

func TestEqualPubEcdsaKeys(t *testing.T) {
	t.Parallel()

	p256 := elliptic.P256()
	p384 := elliptic.P384()

	testCases := []struct {
		name     string
		k1       *ecdsa.PublicKey
		k2       *ecdsa.PublicKey
		expected bool
	}{
		{
			name:     "both nil",
			k1:       nil,
			k2:       nil,
			expected: true,
		},
		{
			name:     "one nil, other non-nil",
			k1:       &ecdsa.PublicKey{Curve: p256, X: big.NewInt(1), Y: big.NewInt(2)},
			k2:       nil,
			expected: false,
		},
		{
			name:     "same curve, same X, same Y",
			k1:       &ecdsa.PublicKey{Curve: p256, X: big.NewInt(1), Y: big.NewInt(2)},
			k2:       &ecdsa.PublicKey{Curve: p256, X: big.NewInt(1), Y: big.NewInt(2)},
			expected: true,
		},
		{
			name:     "different curve",
			k1:       &ecdsa.PublicKey{Curve: p256, X: big.NewInt(1), Y: big.NewInt(2)},
			k2:       &ecdsa.PublicKey{Curve: p384, X: big.NewInt(1), Y: big.NewInt(2)},
			expected: false,
		},
		{
			name:     "same curve, different X",
			k1:       &ecdsa.PublicKey{Curve: p256, X: big.NewInt(1), Y: big.NewInt(2)},
			k2:       &ecdsa.PublicKey{Curve: p256, X: big.NewInt(3), Y: big.NewInt(2)},
			expected: false,
		},
		{
			name:     "same curve, same X, different Y",
			k1:       &ecdsa.PublicKey{Curve: p256, X: big.NewInt(1), Y: big.NewInt(2)},
			k2:       &ecdsa.PublicKey{Curve: p256, X: big.NewInt(1), Y: big.NewInt(3)},
			expected: false,
		},
	}

	for _, tt := range testCases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := equalPubEcdsaKeys(tt.k1, tt.k2)
			assert.Equal(t, tt.expected, result)
		})
	}
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

	rootB64der, err := echelper.PublicEcdsaToB64Der(&ecdsaKeys[0].PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to convert public key to DER: %w", err)
	}

	// The first link is self-signed.
	links[0] = chainLink{
		Data: rootB64der,
		Sig:  nil,
	}

	for i := 0; i < len(ecdsaKeys)-1; i++ {
		parentKey := ecdsaKeys[i]
		childKey := ecdsaKeys[i+1]

		childB64Der, err := echelper.PublicEcdsaToB64Der(&childKey.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("failed to convert public key to DER: %w", err)
		}

		sig, err := echelper.Sign(parentKey, childB64Der)
		if err != nil {
			return nil, fmt.Errorf("failed to sign DER: %w", err)
		}

		links[i+1] = chainLink{
			Data: childB64Der,
			Sig:  sig,
		}
	}

	lastKey := ecdsaKeys[len(ecdsaKeys)-1]

	// sign data with the last key
	sig, err := echelper.Sign(lastKey, data)
	if err != nil {
		return nil, fmt.Errorf("failed to sign data with last key: %w", err)
	}

	links[len(ecdsaKeys)] = chainLink{
		Data: data,
		Sig:  sig,
	}

	return &chain{Links: links}, nil
}
