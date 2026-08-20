package localserver

import (
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ecdsaPubKey(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name          string
		curve         string
		mutate        func(x, y []byte) ([]byte, []byte)
		errorExpected bool
	}{
		{name: "valid P-256", curve: curveP256},
		{name: "valid P-384", curve: curveP384},
		{name: "valid P-521", curve: curveP521},
		{
			name:          "coordinate one byte short",
			curve:         curveP256,
			mutate:        func(x, y []byte) ([]byte, []byte) { return x[1:], y },
			errorExpected: true,
		},
		{
			name:          "coordinate one byte long",
			curve:         curveP256,
			mutate:        func(x, y []byte) ([]byte, []byte) { return append([]byte{0x00}, x...), y },
			errorExpected: true,
		},
		{
			name:          "off-curve point",
			curve:         curveP256,
			mutate:        func(x, y []byte) ([]byte, []byte) { y[len(y)-1] ^= 0xff; return x, y },
			errorExpected: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create a key as described by the test case
			curve, err := parseEllipticCurve(tt.curve)
			require.NoError(t, err)
			key, err := ecdsa.GenerateKey(curve, rand.Reader)
			require.NoError(t, err)
			encoded, err := key.PublicKey.Bytes()
			require.NoError(t, err)

			// Handle mutations (point is off the curve, coordinates are malformed)
			coordinateLen := (curve.Params().BitSize + 7) / 8
			x, y := encoded[1:1+coordinateLen], encoded[1+coordinateLen:]
			if tt.mutate != nil {
				x, y = tt.mutate(x, y)
			}

			// Now, convert to a jwk
			testJwk := &jwk{
				Curve: tt.curve,
				X:     base64.RawURLEncoding.EncodeToString(x),
				Y:     base64.RawURLEncoding.EncodeToString(y),
			}

			// Parse the pubkey
			pubKey, err := testJwk.ecdsaPubKey()

			if tt.errorExpected {
				require.Error(t, err)
				return
			}

			// Confirm we get back the same pubkey that we started with
			require.NoError(t, err)
			require.True(t, key.PublicKey.Equal(pubKey))
		})
	}
}
