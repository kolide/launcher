package packagekit

import (
	"context"
	"testing"

	"github.com/kolide/kit/ulid"
	"github.com/stretchr/testify/require"
)

func TestContextError(t *testing.T) {
	t.Parallel()

	_, err := GetFromContext(context.Background(), ContextNotarizationUuidKey)
	require.Error(t, err)
}

func TestContextBlanks(t *testing.T) {
	t.Parallel()

	ctx := InitContext(context.Background())

	actual, err := GetFromContext(ctx, ContextNotarizationUuidKey)
	require.NoError(t, err)
	require.Empty(t, actual)

}

func TestContext(t *testing.T) {
	t.Parallel()

	ctx := InitContext(context.Background())

	var contextPairs = []struct {
		name string
		key  contextKey
		val  string
	}{
		{
			name: "notarization uuid",
			key:  ContextNotarizationUuidKey,
			val:  ulid.New(),
		},
		{
			name: "launcher version",
			key:  ContextLauncherVersionKey,
			val:  ulid.New(),
		},
		{
			name: "osquery version",
			key:  ContextOsqueryVersionKey,
			val:  ulid.New(),
		},
	}

	for _, pair := range contextPairs {
		setInContext(ctx, pair.key, pair.val)
	}

	for _, pair := range contextPairs {
		t.Run(pair.name, func(t *testing.T) {
			actual, err := GetFromContext(ctx, pair.key)
			require.NoError(t, err)
			require.Equal(t, pair.val, actual)
		})
	}

}
