package knapsack

import (
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/agent/flags"
	storageci "github.com/kolide/launcher/pkg/agent/storage/ci"
	"github.com/stretchr/testify/require"
)

func NewTestingKnapsack(t *testing.T) *Knapsack {
	db := storageci.SetupDB(t)
	stores, err := storageci.MakeStores(t, log.NewNopLogger(), db)
	require.NoError(t, err)
	f := flags.NewFlagController(log.NewNopLogger(), flags.DefaultFlagValues(), nil, nil, nil)
	return New(stores, f, db)
}
