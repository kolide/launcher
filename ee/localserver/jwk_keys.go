package localserver

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
)

type jwk struct {
	Type  string `json:"kty"`
	Curve string `json:"crv"`
	X     string `json:"x"`
	Y     string `json:"y"`
	KeyID string `json:"kid"`
}

// jwkToECDSAPublicKey converts a JWK JSON string into an ECDSA public key.
func (jwk jwk) ecdsaPubKey() (*ecdsa.PublicKey, error) {
	// Ensure the key type is EC.
	if jwk.Type != "EC" {
		return nil, fmt.Errorf("unexpected key type: %s", jwk.Type)
	}

	// Decode the x and y coordinates using base64 URL decoding (unpadded).
	xBytes, err := base64.RawURLEncoding.DecodeString(jwk.X)
	if err != nil {
		return nil, fmt.Errorf("error decoding x coordinate: %v", err)
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(jwk.Y)
	if err != nil {
		return nil, fmt.Errorf("error decoding y coordinate: %v", err)
	}

	// Convert the bytes into big.Int values.
	x := new(big.Int).SetBytes(xBytes)
	y := new(big.Int).SetBytes(yBytes)

	// Determine the elliptic curve based on the crv field.
	var curve elliptic.Curve
	switch jwk.Curve {
	case "P-256":
		curve = elliptic.P256()
	case "P-384":
		curve = elliptic.P384()
	case "P-521":
		curve = elliptic.P521()
	default:
		return nil, fmt.Errorf("unsupported curve: %s", jwk.Curve)
	}

	// Construct the ECDSA public key.
	pubKey := &ecdsa.PublicKey{
		Curve: curve,
		X:     x,
		Y:     y,
	}

	// this is a little weird, but it's the recommended way to validate a public key,
	// under the hood it calls Curve.IsOnCurve(...), but if you call that directly
	// you get deprecated warnings
	if _, err := pubKey.ECDH(); err != nil {
		return nil, fmt.Errorf("invalid public key: %w", err)
	}

	return pubKey, nil
}

// jwkToX25519PublicKey converts jwk in to x25519 key (*[32]byte)
func (jwk jwk) x25519PubKey() (*[32]byte, error) {
	// Decode the "x" coordinate using base64 URL decoding (unpadded).
	xBytes, err := base64.RawURLEncoding.DecodeString(jwk.X)
	if err != nil {
		return nil, fmt.Errorf("error decoding x coordinate: %v", err)
	}

	// X25519 public keys should be 32 bytes.
	if len(xBytes) != 32 {
		return nil, errors.New("invalid x coordinate length for X25519, expected 32 bytes")
	}

	// Copy the bytes into a fixed size array.
	var pubKey [32]byte
	copy(pubKey[:], xBytes)

	return &pubKey, nil
}
