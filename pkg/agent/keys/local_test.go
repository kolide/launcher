package keys

import (
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/agent/storage"
	"github.com/stretchr/testify/require"
)

func TestSetupLocalDbKey(t *testing.T) {
	t.Parallel()

	logger := log.NewNopLogger()
	getset := storage.NewCIKeyValueStore(t, log.NewNopLogger(), bucketName)

	key, err := SetupLocalDbKey(logger, getset)
	require.NoError(t, err)
	require.NotNil(t, key)

	// Call a thing. Make sure this is a real key
	require.NotNil(t, key.Public())

	// If we call this _again_ do we get the same key back?
	key2, err := SetupLocalDbKey(logger, getset)
	require.NoError(t, err)
	require.Equal(t, key.Public(), key2.Public())

}
