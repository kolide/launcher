package storage

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKeyByIdentifier(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName   string
		key            []byte
		identifierType []byte
		identifier     []byte
		expectedKey    []byte
	}{
		{
			testCaseName:   "default identifier",
			key:            []byte("nodeKey"),
			identifierType: IdentifierTypeRegistration,
			identifier:     []byte("default"),
			expectedKey:    []byte("nodeKey"),
		},
		{
			testCaseName:   "empty identifier",
			key:            []byte("config"),
			identifierType: IdentifierTypeRegistration,
			identifier:     nil,
			expectedKey:    []byte("config"),
		},
		{
			testCaseName:   "registration identifier",
			key:            []byte("uuid"),
			identifierType: IdentifierTypeRegistration,
			identifier:     []byte("some-test-registration-id"),
			expectedKey:    []byte("uuid:registration:some-test-registration-id"),
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.expectedKey, KeyByIdentifier(tt.key, tt.identifierType, tt.identifier))
		})
	}
}

func TestSplitKey(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName           string
		key                    []byte
		expectedKey            []byte
		expectedIdentifierType []byte
		expectedIdentifier     []byte
	}{
		{
			testCaseName:           "default node key",
			key:                    []byte("nodeKey"),
			expectedKey:            []byte("nodeKey"),
			expectedIdentifierType: nil,
			expectedIdentifier:     []byte("default"),
		},
		{
			testCaseName:           "uuid by registration",
			key:                    []byte("uuid:registration:some-test-registration-id"),
			expectedKey:            []byte("uuid"),
			expectedIdentifierType: IdentifierTypeRegistration,
			expectedIdentifier:     []byte("some-test-registration-id"),
		},
		{
			testCaseName:           "katc table by registration",
			key:                    []byte("katc_some_test_table:registration:another-test-registration-id"),
			expectedKey:            []byte("katc_some_test_table"),
			expectedIdentifierType: IdentifierTypeRegistration,
			expectedIdentifier:     []byte("another-test-registration-id"),
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			splitKey, identifierType, identifier := SplitKey(tt.key)
			require.Equal(t, tt.expectedKey, splitKey)
			require.Equal(t, tt.expectedIdentifierType, identifierType)
			require.Equal(t, tt.expectedIdentifier, identifier)
		})
	}
}
