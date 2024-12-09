package storage

import "bytes"

var (
	// Well-known keys
	ObservabilityIngestAuthTokenKey = []byte("observability_ingest_auth_token")

	// Identifier types in complex keys
	IdentifierTypeRegistration = []byte("registration")

	defaultIdentifier = []byte("default")
)

const (
	keyDelimiter byte = 58 // :
)

func KeyByIdentifier(key []byte, identifierType []byte, identifier []byte) []byte {
	// The default value is stored under `key`, without any identifier
	if len(identifier) == 0 || bytes.Equal(identifier, defaultIdentifier) {
		return key
	}

	// Key will take the form `<key>:<identifierType>:<identifier>` -- allocate
	// a new key with the appropriate capacity.
	totalSize := len(key) + 1 + len(identifierType) + 1 + len(identifier)
	newKey := make([]byte, 0, totalSize)

	newKey = append(newKey, key...)
	newKey = append(newKey, keyDelimiter)
	newKey = append(newKey, identifierType...)
	newKey = append(newKey, keyDelimiter)
	newKey = append(newKey, identifier...)

	return newKey
}
