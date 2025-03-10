package localserver

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
)

// These are the hardcoded certificates

const (
	k2EccServerCert = `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEmAO4tYINU14/i0LvONs1IXVwaFnF
dNsydDr38XrL29kiFl+vTkp4gVx6172oNSL3KRBQmjMXqWkLNoxXaWS3uQ==
-----END PUBLIC KEY-----`

	reviewEccServerCert = `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEIgYTWPi8N7b0H69tnN543HbjAoLc
GINysvEwYrNoGjASt+nqzlFesagt+2A/4W7JR16nE91mbCHn+HV6x+H8gw==
-----END PUBLIC KEY-----`

	localhostEccServerCert = `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEwowFsPUaOC61LAfDz1hLnsuSDfEx
SC4TSfHtbHHv3lx2/Bfu+H0szXYZ75GF/qZ5edobq3UkABN6OaFnnJId3w==
-----END PUBLIC KEY-----`

	dt4aCertsJWK = `
{
  "ci": {
    "crv": "P-256",
    "kid": "LJDJWSE7WZEVTIWYZXFEGO4UWU",
    "kty": "EC",
    "x": "RqXvX6ByCw5XzYtqvt_xMSJwBaA9aoPH-Mc3yQ2HGhE",
    "y": "RGellUkwDt2v0HUdCqKs8WLBa4bWQ4ZaKPfAesGdjoA"
  },
  "dca": {
    "crv": "P-256",
    "kid": "O7OHTURDFFHHNNV6NIKCPCZG3U",
    "kty": "EC",
    "x": "e4MUHZOCk7Jas0wKLZZj0BTUnfzqilXV7EykZelq8kw",
    "y": "Pep2Saf8RTYH0KaIH4Kh3AcdsgATh8pr2jUBviULwWo"
  },
  "dck": {
    "crv": "P-256",
    "kid": "EQFDLBCEORHJRLZMSUAKAUWEIQ",
    "kty": "EC",
    "x": "J3XQzYvsix7WQV1g-N7IgFA_J-Fja2_R7QcqMr1WU3g",
    "y": "b9YrgvnWeJxjZ5HX2ERau5PcJDKDnrSDcyLJpuNiuMc"
  },
  "deu": {
    "crv": "P-256",
    "kid": "O7OHTURDFFHHNNV6NIKCPCZG3U",
    "kty": "EC",
    "x": "e4MUHZOCk7Jas0wKLZZj0BTUnfzqilXV7EykZelq8kw",
    "y": "Pep2Saf8RTYH0KaIH4Kh3AcdsgATh8pr2jUBviULwWo"
  },
  "dev": {
    "crv": "P-256",
    "kid": "LJDJWSE7WZEVTIWYZXFEGO4UWU",
    "kty": "EC",
    "x": "RqXvX6ByCw5XzYtqvt_xMSJwBaA9aoPH-Mc3yQ2HGhE",
    "y": "RGellUkwDt2v0HUdCqKs8WLBa4bWQ4ZaKPfAesGdjoA"
  },
  "ent": {
    "crv": "P-256",
    "kid": "O7OHTURDFFHHNNV6NIKCPCZG3U",
    "kty": "EC",
    "x": "e4MUHZOCk7Jas0wKLZZj0BTUnfzqilXV7EykZelq8kw",
    "y": "Pep2Saf8RTYH0KaIH4Kh3AcdsgATh8pr2jUBviULwWo"
  },
  "lcl": {
    "crv": "P-256",
    "kid": "LJDJWSE7WZEVTIWYZXFEGO4UWU",
    "kty": "EC",
    "x": "RqXvX6ByCw5XzYtqvt_xMSJwBaA9aoPH-Mc3yQ2HGhE",
    "y": "RGellUkwDt2v0HUdCqKs8WLBa4bWQ4ZaKPfAesGdjoA"
  },
  "pca": {
    "crv": "P-256",
    "kid": "O7OHTURDFFHHNNV6NIKCPCZG3U",
    "kty": "EC",
    "x": "e4MUHZOCk7Jas0wKLZZj0BTUnfzqilXV7EykZelq8kw",
    "y": "Pep2Saf8RTYH0KaIH4Kh3AcdsgATh8pr2jUBviULwWo"
  },
  "peu": {
    "crv": "P-256",
    "kid": "LJDJWSE7WZEVTIWYZXFEGO4UWU",
    "kty": "EC",
    "x": "RqXvX6ByCw5XzYtqvt_xMSJwBaA9aoPH-Mc3yQ2HGhE",
    "y": "RGellUkwDt2v0HUdCqKs8WLBa4bWQ4ZaKPfAesGdjoA"
  },
  "prd": {
    "crv": "P-256",
    "kid": "O7OHTURDFFHHNNV6NIKCPCZG3U",
    "kty": "EC",
    "x": "e4MUHZOCk7Jas0wKLZZj0BTUnfzqilXV7EykZelq8kw",
    "y": "Pep2Saf8RTYH0KaIH4Kh3AcdsgATh8pr2jUBviULwWo"
  },
  "rev": {
    "crv": "P-256",
    "kid": "LJDJWSE7WZEVTIWYZXFEGO4UWU",
    "kty": "EC",
    "x": "RqXvX6ByCw5XzYtqvt_xMSJwBaA9aoPH-Mc3yQ2HGhE",
    "y": "RGellUkwDt2v0HUdCqKs8WLBa4bWQ4ZaKPfAesGdjoA"
  },
  "stg": {
    "crv": "P-256",
    "kid": "O7OHTURDFFHHNNV6NIKCPCZG3U",
    "kty": "EC",
    "x": "e4MUHZOCk7Jas0wKLZZj0BTUnfzqilXV7EykZelq8kw",
    "y": "Pep2Saf8RTYH0KaIH4Kh3AcdsgATh8pr2jUBviULwWo"
  },
  "tca": {
    "crv": "P-256",
    "kid": "LJDJWSE7WZEVTIWYZXFEGO4UWU",
    "kty": "EC",
    "x": "RqXvX6ByCw5XzYtqvt_xMSJwBaA9aoPH-Mc3yQ2HGhE",
    "y": "RGellUkwDt2v0HUdCqKs8WLBa4bWQ4ZaKPfAesGdjoA"
  },
  "teu": {
    "crv": "P-256",
    "kid": "O7OHTURDFFHHNNV6NIKCPCZG3U",
    "kty": "EC",
    "x": "e4MUHZOCk7Jas0wKLZZj0BTUnfzqilXV7EykZelq8kw",
    "y": "Pep2Saf8RTYH0KaIH4Kh3AcdsgATh8pr2jUBviULwWo"
  },
  "tst": {
    "crv": "P-256",
    "kid": "LJDJWSE7WZEVTIWYZXFEGO4UWU",
    "kty": "EC",
    "x": "RqXvX6ByCw5XzYtqvt_xMSJwBaA9aoPH-Mc3yQ2HGhE",
    "y": "RGellUkwDt2v0HUdCqKs8WLBa4bWQ4ZaKPfAesGdjoA"
  }
}`
)

func dt4aKeys() (map[string]*ecdsa.PublicKey, error) {
	dt4aKeyMap := make(map[string]*ecdsa.PublicKey)

	// unmarshall the certificates
	certMap := make(map[string]map[string]string)
	err := json.Unmarshal([]byte(dt4aCertsJWK), &certMap)
	if err != nil {
		return nil, err
	}

	for k, v := range certMap {
		xBytes, err := base64.RawURLEncoding.DecodeString(v["x"])
		if err != nil {
			return nil, err
		}
		x := new(big.Int).SetBytes(xBytes)

		yBytes, err := base64.RawURLEncoding.DecodeString(v["y"])
		if err != nil {
			return nil, err
		}
		y := new(big.Int).SetBytes(yBytes)

		pubKey := &ecdsa.PublicKey{
			Curve: elliptic.P256(),
			X:     x,
			Y:     y,
		}

		// this is a little weird, but it's the recommended way to validate a public key,
		// under the hood it calls Curve.IsOnCurve(...), but if you call that directly
		// you get deprecated warnings
		if _, err := pubKey.ECDH(); err != nil {
			return nil, fmt.Errorf("invalid public key: %w", err)
		}

		dt4aKeyMap[k] = pubKey
	}

	return dt4aKeyMap, nil
}
