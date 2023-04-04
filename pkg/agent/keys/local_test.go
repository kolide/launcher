package keys

import (
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/agent/storage"
	storageci "github.com/kolide/launcher/pkg/agent/storage/ci"
	"github.com/stretchr/testify/require"
)

func TestSetupLocalDbKey(t *testing.T) {
	t.Parallel()

	logger := log.NewNopLogger()
	store, err := storageci.NewStore(t, log.NewNopLogger(), storage.ConfigStore.String())
	require.NoError(t, err)

	key, err := SetupLocalDbKey(logger, store)
	require.NoError(t, err)
	require.NotNil(t, key)

	// Call a thing. Make sure this is a real key
	require.NotNil(t, key.Public())

	// If we call this _again_ do we get the same key back?
	key2, err := SetupLocalDbKey(logger, store)
	require.NoError(t, err)
	require.Equal(t, key.Public(), key2.Public())
}
