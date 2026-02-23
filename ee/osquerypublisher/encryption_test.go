package osquerypublisher

import (
	"encoding/base64"
	"testing"

	"github.com/cloudflare/circl/hpke"
	"github.com/stretchr/testify/require"
)

func TestParseKeyData(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		input       string
		expectError bool
		expectedID  string
	}{
		{
			name:        "valid PSK",
			input:       "psk123:" + base64.StdEncoding.EncodeToString([]byte("test-psk!!!!!!!!!!!!!!!!!!!!!!")),
			expectError: false,
			expectedID:  "psk123",
		},
		{
			name:        "missing delimiter",
			input:       "psk123",
			expectError: true,
		},
		{
			name:        "invalid base64",
			input:       "psk123:not-valid-base64!@#",
			expectError: true,
		},
		{
			name:        "empty key ID",
			input:       ":" + base64.StdEncoding.EncodeToString([]byte("test")),
			expectError: true,
		},
		{
			name:        "empty key data",
			input:       "pubkey123:",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := parseKeyData(tt.input)

			if tt.expectError {
				require.Error(t, err)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, tt.expectedID, result.KeyID)
				require.NotEmpty(t, result.Key)
			}
		})
	}
}

func TestEncryptWithHPKE(t *testing.T) {
	t.Parallel()

	// Generate a test HPKE keypair
	suite := hpke.NewSuite(hpke.KEM_X25519_HKDF_SHA256, hpke.KDF_HKDF_SHA256, hpke.AEAD_AES256GCM)
	kemID, _, _ := suite.Params()
	kemScheme := kemID.Scheme()
	pkR, skR, err := kemScheme.GenerateKeyPair()
	require.NoError(t, err)

	// Marshal the public key
	pkRBytes, err := pkR.MarshalBinary()
	require.NoError(t, err)

	// Create test data
	hpkeKey := &KeyData{
		Key:   pkRBytes,
		KeyID: "test-key-id",
	}

	psk := &KeyData{
		Key:   []byte("test-psk-sdghsldfosidfsfgsidfsifdsio"),
		KeyID: "test-psk-id",
	}

	plaintext := []byte("test plaintext message")

	// Encrypt
	encryptedBlob, err := encryptWithHPKE(plaintext, hpkeKey, psk)
	require.NoError(t, err)
	require.NotNil(t, encryptedBlob)

	// Verify encrypted blob structure
	require.Equal(t, currentEncryptedBlobVersion, encryptedBlob.Version)
	require.Equal(t, hpkeKey.KeyID, encryptedBlob.HPKEKeyID)
	require.Equal(t, psk.KeyID, encryptedBlob.PSKID)
	require.NotEmpty(t, encryptedBlob.EncapsulatedKey)
	require.NotEmpty(t, encryptedBlob.Ciphertext)

	// Decrypt to verify round-trip (using the private key we generated)
	encapsulatedKeyBytes, err := base64.StdEncoding.DecodeString(encryptedBlob.EncapsulatedKey)
	require.NoError(t, err, "encapsulated key should be valid base64")

	ciphertextBytes, err := base64.StdEncoding.DecodeString(encryptedBlob.Ciphertext)
	require.NoError(t, err, "ciphertext should be valid base64")

	receiver, err := suite.NewReceiver(skR, []byte(hpkeDomain))
	require.NoError(t, err)

	opener, err := receiver.SetupPSK(encapsulatedKeyBytes, psk.Key, []byte(psk.KeyID))
	require.NoError(t, err)

	decrypted, err := opener.Open(ciphertextBytes, nil)
	require.NoError(t, err)
	require.Equal(t, plaintext, decrypted, "decrypted plaintext should match original")
}

func TestEncryptWithHPKE_EmptyPlaintext(t *testing.T) {
	t.Parallel()

	// Generate a test HPKE keypair
	suite := hpke.NewSuite(hpke.KEM_X25519_HKDF_SHA256, hpke.KDF_HKDF_SHA256, hpke.AEAD_AES256GCM)
	kemID, _, _ := suite.Params()
	kemScheme := kemID.Scheme()
	_, pkR, err := kemScheme.GenerateKeyPair()
	require.NoError(t, err)

	pkRBytes, err := pkR.MarshalBinary()
	require.NoError(t, err)

	hpkeKey := &KeyData{
		Key:   pkRBytes,
		KeyID: "test-key-id",
	}

	psk := &KeyData{
		Key:   []byte("test-psk-32-bytes-long!!"),
		KeyID: "test-psk-id",
	}

	// Encrypt empty plaintext
	encryptedBlob, err := encryptWithHPKE([]byte{}, hpkeKey, psk)
	require.NoError(t, err)
	require.NotNil(t, encryptedBlob)
	require.NotEmpty(t, encryptedBlob.Ciphertext, "even empty plaintext should produce ciphertext")
}
