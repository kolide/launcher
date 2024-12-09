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
