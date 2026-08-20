package localserver

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/base64"
	"errors"
	"fmt"
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

// ecdsaPubKey converts jwk into ecdsa public key
func (j *jwk) ecdsaPubKey() (*ecdsa.PublicKey, error) {
	curve, err := parseEllipticCurve(j.Curve)
	if err != nil {
		return nil, err
	}

	// We can't construct an ecdsa.PublicKey using j.X and j.Y directly, because direct access
	// to those fields has been deprecated. Instead, we construct the SEC1 uncompressed
	// encoding inside a buffer, and then pass that off to the ecdsa package to parse.
	// See https://www.secg.org/sec1-v2.pdf 2.3.3.

	// First, decode the x and y coordinates using base64 URL decoding (unpadded).
	xBytes, err := base64.RawURLEncoding.DecodeString(j.X)
	if err != nil {
		return nil, fmt.Errorf("error decoding x coordinate: %v", err)
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(j.Y)
	if err != nil {
		return nil, fmt.Errorf("error decoding y coordinate: %v", err)
	}

	// Next, figure out how many bytes we need for each coordinate, so we know how many bytes
	// we need in the buffer. curve.Params().BitSize gives us the bitsize for the curve; we
	// add 7 so that we don't miss any additional data for curve sizes that don't divide into 8
	// evenly (e.g. for P-521, 521 / 8 = 65.125, which rounds to 65, so we may be missing data).
	// This is how crypto/elliptic performs this calculation. We expect 32 for P-256, 48 for P-384,
	// 66 for P-521.
	coordinateLen := (curve.Params().BitSize + 7) / 8
	if len(xBytes) != coordinateLen || len(yBytes) != coordinateLen {
		return nil, fmt.Errorf("unexpected coordinate size: expected %d, got X len %d and Y len %d", coordinateLen, len(xBytes), len(yBytes))
	}

	// Now, construct the buffer:
	// The first byte 0x04 indicates that point compression is off; then follows all of the
	// bytes in X; then follows all of the bytes in Y.
	buf := make([]byte, 1+2*coordinateLen)
	buf[0] = 0x04
	for i := range coordinateLen {
		buf[i+1] = xBytes[i]
		buf[i+1+coordinateLen] = yBytes[i]
	}

	// Finally, parse the pubkey. This will also validate that the key is on the curve.
	pubKey, err := ecdsa.ParseUncompressedPublicKey(curve, buf)
	if err != nil {
		return nil, fmt.Errorf("invalid ECDSA public key: %w", err)
	}

	return pubKey, nil
}

// x25519PubKey converts jwk into x25519 key (*[32]byte)
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
