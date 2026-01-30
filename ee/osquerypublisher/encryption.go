package osquerypublisher

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/cloudflare/circl/hpke"
)

const (
	// keyDelimiter is used to separate key ID from key material in concatenated strings
	keyDelimiter = ":"
	currentEncryptedBlobVersion int = 1
)

// KeyData holds a key and it's corresponding identifier.
// this is used to hold both HPKE public keys and PSKs.
type KeyData struct {
	Key   []byte
	KeyID string
}

// EncryptedBlob represents the encrypted payload format as specified in the RFD
type EncryptedBlob struct {
	Version         int    `json:"version"`
	HPKEKeyID       string `json:"hpke_key_id"`
	PSKID           string `json:"psk_id"`
	EncapsulatedKey string `json:"encapsulated_key"` // base64 encoded
	Ciphertext      string `json:"ciphertext"`       // base64 encoded
}

// parseKeyData parses a concatenated string of "keyID:b64(keyMaterial)" into KeyData
// This expects that the key material is base64 encoded in the stored string
func parseKeyData(concatenated string) (*KeyData, error) {
	parts := strings.SplitN(concatenated, keyDelimiter, 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid key data format: expected 'keyID:Key', got %d parts", len(parts))
	}

	keyID := parts[0]
	keyB64 := parts[1]

	if keyID == "" {
		return nil, fmt.Errorf("invalid key data format: key ID is empty")
	}

	if keyB64 == "" {
		return nil, fmt.Errorf("invalid key data format: key data is empty")
	}

	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, fmt.Errorf("failed to b64 decode key: %w", err)
	}

	return &KeyData{
		Key:   key,
		KeyID: keyID,
	}, nil
}

// encryptWithHPKE encrypts plaintext using HPKE with PSK mode
// Uses X25519 KEM, HKDF-SHA256 KDF, and AES-256-GCM AEAD
func encryptWithHPKE(plaintext []byte, hpkeKey *KeyData, psk *KeyData) (*EncryptedBlob, error) {
	kemID := hpke.KEM_X25519_HKDF_SHA256
	suite := hpke.NewSuite(kemID, hpke.KDF_HKDF_SHA256, hpke.AEAD_AES256GCM)

	// parse the public key
	pkR, err := kemID.Scheme().UnmarshalBinaryPublicKey(hpkeKey.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal HPKE public key: %w", err)
	}

	// create sender with recipient's public key.
	// info parameter is empty for now (can be extended with metadata if needed)
	sender, err := suite.NewSender(pkR, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HPKE sender: %w", err)
	}

	// setup PSK mode encryption- this generates an ephemeral keypair and returns the encapsulated key and sealer
	encapsulatedKey, sealer, err := sender.SetupPSK(rand.Reader, psk.Key, []byte(psk.KeyID))
	if err != nil {
		return nil, fmt.Errorf("failed to setup HPKE PSK encryption: %w", err)
	}

	// encrypt the plaintext- associated data (aad) is nil for now (can be extended with metadata if needed)
	ciphertext, err := sealer.Seal(plaintext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt plaintext: %w", err)
	}

	// b64 encode the outputs
	encapsulatedKeyB64 := base64.StdEncoding.EncodeToString(encapsulatedKey)
	ciphertextB64 := base64.StdEncoding.EncodeToString(ciphertext)

	return &EncryptedBlob{
		Version:         currentEncryptedBlobVersion,
		HPKEKeyID:       hpkeKey.KeyID,
		PSKID:           psk.KeyID,
		EncapsulatedKey: encapsulatedKeyB64,
		Ciphertext:      ciphertextB64,
	}, nil
}
