package keys

import (
	"testing"

	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func TestSetupLocalDbKey(t *testing.T) {
	t.Parallel()

	slogger := multislogger.NewNopLogger()
	store, err := storageci.NewStore(t, slogger, storage.ConfigStore.String())
	require.NoError(t, err)

	key, err := SetupLocalDbKey(slogger, store)
	require.NoError(t, err)
	require.NotNil(t, key)

	// Call a thing. Make sure this is a real key
	require.NotNil(t, key.Public())

	// If we call this _again_ do we get the same key back?
	key2, err := SetupLocalDbKey(slogger, store)
	require.NoError(t, err)
	require.Equal(t, key.Public(), key2.Public())
}
