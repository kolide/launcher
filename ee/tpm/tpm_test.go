//go:build !darwin
// +build !darwin

package tpm

import (
	"crypto/rsa"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSealUnseal(t *testing.T) {
	t.Parallel()

	message := []byte("message to be sealed")

	sealed, err := Seal(message)
	require.NoError(t, err)

	// sanity check that the message changed
	require.NotEqual(t, message, sealed)

	unsealed, err := Unseal(sealed)
	require.NoError(t, err)

	require.Equal(t, message, unsealed)
}

func TestSealMaxSize(t *testing.T) {
	t.Parallel()

	_, err := Seal(make([]byte, MaxSealBytes+1))
	require.Error(t, err)
}

func TestSignVerify(t *testing.T) {
	t.Parallel()

	message := []byte("message to be signed and verified")

	signed, err := Sign(message)
	require.NoError(t, err)

	// sanity check that the message changed
	require.NotEqual(t, message, signed)

	pubKey, err := PublicSigningKey()
	require.NoError(t, err)

	hash := CryptoHash.New()
	hash.Write(message)

	require.NoError(t, rsa.VerifyPKCS1v15(pubKey, CryptoHash, hash.Sum(nil), signed))
}
