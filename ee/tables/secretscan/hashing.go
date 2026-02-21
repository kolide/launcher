package secretscan

import (
	"encoding/base64"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
)

// Configurable parameters for Argon2id. RFC 9106 Section 7.3 recommends specific values.
const (
	argonTimeCost   uint32 = 2         // Number of iterations
	argonMemoryCost uint32 = 16 * 1024 // Amount of memory in KiB (~32 MB)
	argonThreads    uint8  = 4         // Number of threads/lanes for parallelism
	argonKeyLength  uint32 = 32        // Length of the generated hash key in bytes
	argonSaltLength uint32 = 16        // Length of the random salt in bytes
	// Truncated hash length for deduplication (reduces exposure risk)
	argonTruncatedLength = 15 // 15 bytes, because it base64s nicely
)

// generateArgon2idHash takes a secret and generates a truncated argon2id hash.
// Because it's used to deduplicate secrets server side, we need a consistent hash.
// To reduce the risk of secret exposure, we truncate the hash.
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
	fullHash := argon2.IDKey([]byte(secret), saltBytes, argonTimeCost, argonMemoryCost, argonThreads, argonKeyLength)

	// Truncate the hash to reduce exposure risk
	truncatedHash := fullHash[:argonTruncatedLength]

	// Encode both to Base64 for storage
	b64Hash := base64.RawStdEncoding.EncodeToString(truncatedHash)

	return b64Hash, nil
}
