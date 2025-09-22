package localserver

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// jwk is a JSON Web Key (JWK) structure for representing public keys,
// this a partial implementation using the stdlib for only the bits we care about,
// RFC https://datatracker.ietf.org/doc/html/rfc7517
type jwk struct {
	Curve string `json:"crv"`
	X     string `json:"x"`
	Y     string `json:"y"`
	KeyID string `json:"kid"`
}

const (
	curveP256 string = "P-256"
	curveP384 string = "P-384"
	curveP521 string = "P-521"
)

func parseEllipticCurve(str string) (elliptic.Curve, error) {
	switch strings.ToUpper(str) {
	case curveP256:
		return elliptic.P256(), nil
	case curveP384:
		return elliptic.P384(), nil
	case curveP521:
		return elliptic.P521(), nil
	default:
		return &elliptic.CurveParams{}, fmt.Errorf("unsupported curve: %s", str)
	}
}

// ecdsaPubKey converts jwk in to ecdsa public key
func (j *jwk) ecdsaPubKey() (*ecdsa.PublicKey, error) {
	curve, err := parseEllipticCurve(j.Curve)
	if err != nil {
		return nil, err
	}

	// Decode the x and y coordinates using base64 URL decoding (unpadded).
	xBytes, err := base64.RawURLEncoding.DecodeString(j.X)
	if err != nil {
		return nil, fmt.Errorf("error decoding x coordinate: %v", err)
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(j.Y)
	if err != nil {
		return nil, fmt.Errorf("error decoding y coordinate: %v", err)
	}

	// Convert the bytes into big.Int values.
	x := new(big.Int).SetBytes(xBytes)
	y := new(big.Int).SetBytes(yBytes)

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

// x25519PubKey converts jwk in to x25519 key (*[32]byte)
func (j *jwk) x25519PubKey() (*[32]byte, error) {
	// Decode the "x" coordinate using base64 URL decoding (unpadded).
	xBytes, err := base64.RawURLEncoding.DecodeString(j.X)
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
