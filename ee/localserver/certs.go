package localserver

import (
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
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
    "LJDJWSE7WZEVTIWYZXFEGO4UWU": {
        "crv": "P-256",
        "kid": "LJDJWSE7WZEVTIWYZXFEGO4UWU",
        "kty": "EC",
        "x": "RqXvX6ByCw5XzYtqvt_xMSJwBaA9aoPH-Mc3yQ2HGhE",
        "y": "RGellUkwDt2v0HUdCqKs8WLBa4bWQ4ZaKPfAesGdjoA"
    },
    "O7OHTURDFFHHNNV6NIKCPCZG3U": {
        "crv": "P-256",
        "kid": "O7OHTURDFFHHNNV6NIKCPCZG3U",
        "kty": "EC",
        "x": "e4MUHZOCk7Jas0wKLZZj0BTUnfzqilXV7EykZelq8kw",
        "y": "Pep2Saf8RTYH0KaIH4Kh3AcdsgATh8pr2jUBviULwWo"
    },
    "EQFDLBCEORHJRLZMSUAKAUWEIQ": {
        "crv": "P-256",
        "kid": "EQFDLBCEORHJRLZMSUAKAUWEIQ",
        "kty": "EC",
        "x": "J3XQzYvsix7WQV1g-N7IgFA_J-Fja2_R7QcqMr1WU3g",
        "y": "b9YrgvnWeJxjZ5HX2ERau5PcJDKDnrSDcyLJpuNiuMc"
    },
    "PCA7GIDALBFYZCWK4MS3G4IXEU": {
        "crv": "P-256",
        "kid": "PCA7GIDALBFYZCWK4MS3G4IXEU",
        "kty": "EC",
        "x": "HMLGLwgkb9NY_x3br_nIhQxkj-XzUcEue6NsPWyAe0g",
        "y": "Agd56jMwBjKEHSmXDlII5fSrYGJcEZQXj_W8xhcp5Bg"
    },
    "XTZV3GJH5NDA3GN5TYYHQS2LY4": {
        "crv": "P-256",
        "kid": "XTZV3GJH5NDA3GN5TYYHQS2LY4",
        "kty": "EC",
        "x": "fJoVe2dIOdHfbqUF_e0Jx1DdWwx1qawdN7KV_gS1lgc",
        "y": "VSrX5h6qrHm0KhfJeX_sR73AsSGDuYMq6wd36eK9zr0"
    },
    "SZ5KQFFBRBAJ7BWXMQK6SNT7VY": {
        "crv": "P-256",
        "kid": "SZ5KQFFBRBAJ7BWXMQK6SNT7VY",
        "kty": "EC",
        "x": "yh2yfAiySH1by8-Zys0fDoxWxLvKzsBidjVv7H-JOQs",
        "y": "yvwU9oucAqdbR-Cnu6AM3Wo8eH1rA14qs9GKZGrcBDM"
    },
    "CGCIR2MFPJG53ANG3UZQHOARUE": {
        "crv": "P-256",
        "kid": "CGCIR2MFPJG53ANG3UZQHOARUE",
        "kty": "EC",
        "x": "kqCHC5TWP0AfB-Is4YqkuG01yHXr90MRIHsWl5cA1bo",
        "y": "O7Dr6cV5c48f3s-7b7nC9emlhrpZaQKGbW0p_fgO12o"
    },
    "EVNNXZX7ARDXTHVDPIIH7UBLIM": {
        "crv": "P-256",
        "kid": "EVNNXZX7ARDXTHVDPIIH7UBLIM",
        "kty": "EC",
        "x": "UYlZvyQwqDdth2C0EdgB7aQz7oXxfft2GwxfsNoeq1Q",
        "y": "9HRC4q-57_VjEl-BxLj8xCdSq8-GRCq9APc9b48RiEg"
    },
    "UQB2V5K4SVHKXJYDK52ESEOITU": {
        "crv": "P-256",
        "kid": "UQB2V5K4SVHKXJYDK52ESEOITU",
        "kty": "EC",
        "x": "iGmuezNq8h1xFrctHA4IZ8w4uNs8FnEGM2H58-u9rLg",
        "y": "fdnegLFLCEEFLxlQIirUnLLBC3W3hiWdSdcoyfezWmc"
    },
    "CDTTGMTBNZDKNLJJRVGZG5EJWI": {
        "crv": "P-256",
        "kid": "CDTTGMTBNZDKNLJJRVGZG5EJWI",
        "kty": "EC",
        "x": "dpfhbnwobCJ1mn67aa-MtpK_HWGgACP2QIRi5sIQHLw",
        "y": "AI6PPzaV80SlaT2YJpO0jR-s_p6V4l3mE21_HmN8btU"
    },
    "QKTPGJABLNC67JF7LNBYA67H6U": {
        "crv": "P-256",
        "kid": "QKTPGJABLNC67JF7LNBYA67H6U",
        "kty": "EC",
        "x": "K_iuK_QTiyUGKnNwQawfNNZaK2r_74LPJ-Dh8E3q2Eg",
        "y": "cQtwUMjR1v6yCUkuJUQ8giG8i0094EjBgI4ZvBSlu7I"
    },
    "EIXVCJMWU5EOHO7KFZRTXJFDPM": {
        "crv": "P-256",
        "kid": "EIXVCJMWU5EOHO7KFZRTXJFDPM",
        "kty": "EC",
        "x": "m41RPfNro2eTO4QvrlcS4YvFGyfSipKYSWvxFCJmd9s",
        "y": "-LcFdVaFWLKpQwxIJtg_Wbt96nSW6UflcHFccukyfYY"
    },
    "4XAYQZYLERCODIA466B6N5CVTU": {
        "crv": "P-256",
        "kid": "4XAYQZYLERCODIA466B6N5CVTU",
        "kty": "EC",
        "x": "ToQ1gpeYXel8NOj_nPaxoQWnTWPYFdSLFu9WyRPSeCI",
        "y": "6I1ce5IWh-sShpy6zWuEPe1xfxXfZKGPKLkjmgFlF8g"
    },
    "QZ4LYXGF2FBPVPOUXOB6JRG7FU": {
        "crv": "P-256",
        "kid": "QZ4LYXGF2FBPVPOUXOB6JRG7FU",
        "kty": "EC",
        "x": "3UcmjOw5gySrmdRhPi79uPrBc4wMK82lk_2ZjHhd8AM",
        "y": "9KyEHTauwzUZMhO9bbfdLXgBurzik_U6hOAvAdEmMOY"
    },
    "XEEA7W43BVHCNG3GXWOKXOCQUI": {
        "crv": "P-256",
        "kid": "XEEA7W43BVHCNG3GXWOKXOCQUI",
        "kty": "EC",
        "x": "9NunJuqdPEvmXid4XykAvMT7oVD2VoJAesvt-OGyVqI",
        "y": "c3A92ku97Gs_BU0ADNl0lGJ-h0TFvShYZgNQMMBf0T0"
    },
    "PUIDTW7VXFHN5CV6DBY4KVTIFE": {
        "crv": "P-256",
        "kid": "PUIDTW7VXFHN5CV6DBY4KVTIFE",
        "kty": "EC",
        "x": "8Y6AMk7wwVZfzqqGbFIdRWKQVm9mvjX1M-4lp7LBBGw",
        "y": "uYFFX6PSycR-7EHSy1LXllg44tWZ5iFGfHJplIWcmYQ"
    }
}`
)

func dt4aKeys() (map[string]*ecdsa.PublicKey, error) {
	dt4aKeyMap := make(map[string]*ecdsa.PublicKey)

	certMap := make(map[string]json.RawMessage)
	if err := json.Unmarshal([]byte(dt4aCertsJWK), &certMap); err != nil {
		return nil, err
	}

	for k, v := range certMap {

		var j jwk
		if err := json.Unmarshal(v, &j); err != nil {
			return nil, fmt.Errorf("failed to unmarshal JWK: %w", err)
		}

		pubKey, err := j.ecdsaPubKey()
		if err != nil {
			return nil, fmt.Errorf("failed to convert JWK to ECDSA public key: %w", err)
		}

		dt4aKeyMap[k] = pubKey
	}

	return dt4aKeyMap, nil
}
