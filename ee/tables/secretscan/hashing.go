package secretscan

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
)

// Configurable parameters for Argon2id. RFC 9106 Section 7.3 recommends specific values.
const (
	argonTimeCost   uint32 = 2         // Number of iterations
	argonMemoryCost uint32 = 16 * 1024 // Amount of memory in KiB (~32 MB)
	argonThreads    uint8  = 4         // Number of threads/lanes for parallelism
	argonKeyLength  uint32 = 5         // Length of the generated hash key, in bytes. This number is sensitive. See PS-267
	argonSaltLength uint32 = 16        // Length of the random salt in bytes
)

// generateArgon2idHash takes a secret and salt, and generates an argon2id hash.
// salt must be provided, and is expected to be unique per org
func generateArgon2idHash(secret string, base64Salt string) (hash string, err error) {
	if len(base64Salt) == 0 {
		return "", errors.New("must provide a salt")
	}

	saltBytes, err := base64.StdEncoding.DecodeString(base64Salt)
	if err != nil {
		return "", fmt.Errorf("decoding salt: %w", err)
	}

	if uint32(len(saltBytes)) != argonSaltLength {
		return "", fmt.Errorf("salt should be %d bytes, but is %d", argonSaltLength, len(saltBytes))
	}

	// Generate the full Argon2id hash
	argonHash := argon2.IDKey([]byte(secret), saltBytes, argonTimeCost, argonMemoryCost, argonThreads, argonKeyLength)

	// Encode to string
	encodedHash := hex.EncodeToString(argonHash)

	return encodedHash, nil
}
