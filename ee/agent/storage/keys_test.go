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
			identifierType: IdentifierTypeEnrollment,
			identifier:     []byte("default"),
			expectedKey:    []byte("nodeKey"),
		},
		{
			testCaseName:   "empty identifier",
			key:            []byte("config"),
			identifierType: IdentifierTypeEnrollment,
			identifier:     nil,
			expectedKey:    []byte("config"),
		},
		{
			testCaseName:   "enrollment identifier",
			key:            []byte("uuid"),
			identifierType: IdentifierTypeEnrollment,
			identifier:     []byte("some-test-enrollment-id"),
			expectedKey:    []byte("uuid:registration:some-test-enrollment-id"),
		},
	} {
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
			testCaseName:           "uuid by enrollment",
			key:                    []byte("uuid:registration:some-test-enrollment-id"),
			expectedKey:            []byte("uuid"),
			expectedIdentifierType: IdentifierTypeEnrollment,
			expectedIdentifier:     []byte("some-test-enrollment-id"),
		},
		{
			testCaseName:           "katc table by enrollment",
			key:                    []byte("katc_some_test_table:registration:another-test-enrollment-id"),
			expectedKey:            []byte("katc_some_test_table"),
			expectedIdentifierType: IdentifierTypeEnrollment,
			expectedIdentifier:     []byte("another-test-enrollment-id"),
		},
	} {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			splitKey, identifierType, identifier := SplitKey(tt.key)
			require.Equal(t, tt.expectedKey, splitKey)
			require.Equal(t, tt.expectedIdentifierType, identifierType)
			require.Equal(t, tt.expectedIdentifier, identifier)
		})
	}
}
