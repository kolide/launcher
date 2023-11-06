package dataflatten

import (
	"fmt"
	"io"
	"os"

	"github.com/golang-jwt/jwt/v5"
)

// JWTFile adds support for the kolide_jwt table, which allows parsing
// a file containing a JWT. Note that the kolide_jwt table does not handle
// verification - this is a utility table for convenience.
func JWTFile(file string, opts ...FlattenOpts) ([]Row, error) {
	return flattenJWT(file, opts...)
}

func flattenJWT(path string, opts ...FlattenOpts) ([]Row, error) {
	// for now, make it clear that any data we parse is unverified
	results := map[string]interface{}{"verified": false}

	jwtFH, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("unable to access file: %w", err)
	}

	defer jwtFH.Close()

	tokenRaw, err := io.ReadAll(jwtFH)
	if err != nil {
		return nil, fmt.Errorf("unable to read JWT: %w", err)
	}

	// attempt decode into the generic (default) MapClaims struct to ensure we capture
	// any claims data that might be useful
	token, _, err := new(jwt.Parser).ParseUnverified(string(tokenRaw), jwt.MapClaims{})
	if err != nil {
		return nil, fmt.Errorf("unable to parse JWT: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("JWT has no parseable claims")
	}

	parsedClaims := map[string]interface{}{}
	for k, v := range claims {
		parsedClaims[k] = v
	}

	results["claims"] = parsedClaims
	results["header"] = token.Header

	return Flatten(results, opts...)
}
