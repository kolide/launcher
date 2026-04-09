package osquerypublisher

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudflare/circl/hpke"
)

const (
	// keyDelimiter is used to separate key ID from key material in concatenated strings
	keyDelimiter                string = ":"
	currentEncryptedBlobVersion int    = 1
	hpkeDomain                  string = "AGENT_INGESTER_UPLOAD_ENC_V1"
	// the payloadAAD stays unencrypted and is used to authenticate the payload to ensure the message hasn't
	// been tampered with in transit. The receiver/decrypter must provide the same value in their decryption flow.
	// Using the encryption suite is common practice for this type of data, but it is only important that whatever the value
	// is cryptographically authenticated. If we chose to use a new suite and wanted to update this value, we should bump the
	// EncryptedBlob version so that the receiver knows to utilize a newer AAD value.
	payloadAAD  string = "HPKE-PSK-X25519-HKDF-SHA256-AES-256-GCM"
	metadataAAD string = "HPKE-BASE-X25519-HKDF-SHA256-AES-256-GCM"
)

// KeyData holds a key and it's corresponding identifier.
// this is used to hold both HPKE public keys and PSKs.
type KeyData struct {
	Key   []byte
	KeyID string
}

// EncryptedBlob represents the encrypted payload format
type EncryptedBlob struct {
	Version         int    `json:"version"`
	HPKEKeyID       string `json:"hpke_key_id"`
	PSKID           string `json:"psk_id"`
	EncapsulatedKey string `json:"encapsulated_key"` // base64 encoded (PSK-mode HPKE for payload)
	Ciphertext      string `json:"ciphertext"`       // base64 encoded
	// Metadata uses HPKE base mode (separate handshake from the PSK payload)
	MetadataEncapsulatedKey string `json:"metadata_encapsulated_key"` // base64 encoded
	MetadataCiphertext      string `json:"metadata_ciphertext"`       // base64 encoded
}

// Metadata represents the metadata required for data routing
type Metadata struct {
	DeviceID       string `json:"device_id"`
	OrganizationID string `json:"organization_id"`
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
		return nil, errors.New("invalid key data format: key ID is empty")
	}

	if keyB64 == "" {
		return nil, errors.New("invalid key data format: key data is empty")
	}

	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, fmt.Errorf("failed to b64 decode key: %w", err)
	}

	return &KeyData{
		KeyID: keyID,
		Key:   key,
	}, nil
}

// encryptWithHPKE encrypts plaintext in HPKE PSK mode and metadataJSON in a separate HPKE base-mode
// handshake (same KEM/KDF/AEAD: X25519, HKDF-SHA256, AES-256-GCM). metadataJSON must be non-empty.
func encryptWithHPKE(plaintext []byte, hpkeKey *KeyData, psk *KeyData, metadataJSON []byte) (*EncryptedBlob, error) {
	if len(metadataJSON) == 0 {
		return nil, errors.New("metadata JSON is empty")
	}
	kemID := hpke.KEM_X25519_HKDF_SHA256
	suite := hpke.NewSuite(kemID, hpke.KDF_HKDF_SHA256, hpke.AEAD_AES256GCM)

	// parse the public key
	pkR, err := kemID.Scheme().UnmarshalBinaryPublicKey(hpkeKey.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal HPKE public key: %w", err)
	}

	// create sender with recipient's public key, adding the hpkeDomain string as the information parameter
	sender, err := suite.NewSender(pkR, []byte(hpkeDomain))
	if err != nil {
		return nil, fmt.Errorf("failed to create HPKE sender: %w", err)
	}

	// setup PSK mode encryption- this generates an ephemeral keypair and returns the encapsulated key and sealer
	encapsulatedKey, sealer, err := sender.SetupPSK(rand.Reader, psk.Key, []byte(psk.KeyID))
	if err != nil {
		return nil, fmt.Errorf("failed to setup HPKE PSK encryption: %w", err)
	}

	// encrypt the plaintext with the associated data (aad). The aad should include any information
	// that should be cryptographically authenticated but can be available in plaintext to the receiver.
	ciphertext, err := sealer.Seal(plaintext, []byte(payloadAAD))
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt plaintext: %w", err)
	}

	// b64 encode the outputs
	encapsulatedKeyB64 := base64.StdEncoding.EncodeToString(encapsulatedKey)
	ciphertextB64 := base64.StdEncoding.EncodeToString(ciphertext)

	metaSender, err := suite.NewSender(pkR, []byte(hpkeDomain))
	if err != nil {
		return nil, fmt.Errorf("failed to create HPKE sender for metadata: %w", err)
	}
	metadataEncapsulatedKey, metadataSealer, err := metaSender.Setup(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to setup HPKE base encryption for metadata: %w", err)
	}
	metadataCiphertext, err := metadataSealer.Seal(metadataJSON, []byte(metadataAAD))
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt metadata: %w", err)
	}
	metadataEncapsulatedKeyB64 := base64.StdEncoding.EncodeToString(metadataEncapsulatedKey)
	metadataCiphertextB64 := base64.StdEncoding.EncodeToString(metadataCiphertext)

	return &EncryptedBlob{
		Version:                 currentEncryptedBlobVersion,
		HPKEKeyID:               hpkeKey.KeyID,
		PSKID:                   psk.KeyID,
		EncapsulatedKey:         encapsulatedKeyB64,
		Ciphertext:              ciphertextB64,
		MetadataEncapsulatedKey: metadataEncapsulatedKeyB64,
		MetadataCiphertext:      metadataCiphertextB64,
	}, nil
}
