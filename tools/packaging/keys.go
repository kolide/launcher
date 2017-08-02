package packaging

import (
	"crypto/md5"
	"fmt"
	"strings"

	jwt "github.com/dgrijalva/jwt-go"
)

// enrollSecret will generate an enrollment secret for a tenant given a valid
// signing key
func enrollSecret(tenantName string, pemKey []byte) (string, error) {
	fingerPrint := fmt.Sprintf("% x", md5.Sum([]byte(pemKey)))
	fingerPrint = strings.Replace(fingerPrint, " ", ":", 15)

	var claims = struct {
		Tenant string `json:"tenant"`
		KID    string `json:"kid"`
		jwt.StandardClaims
	}{
		Tenant: tenantName,
		KID:    fingerPrint,
	}

	key, err := jwt.ParseRSAPrivateKeyFromPEM(pemKey)
	if err != nil {
		return "", fmt.Errorf("parsing pem key: %s", err)
	}
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodRS256, &claims)
	signed, err := jwtToken.SignedString(key)
	if err != nil {
		return "", err
	}
	return signed, nil
}
