package simulator

import (
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"
)

func TestFunctionalOptions(t *testing.T) {
	t.Parallel()

	simulation := createSimulationRuntime(
		log.NewNopLogger(),
		nil, "", "",
		WithInsecure(),
	)

	// verify the functional options are correct
	require.True(t, simulation.insecure)
	require.False(t, simulation.insecureGrpc)

	// we haven't started the simulation yet, so the instance should think it's
	// healthy still
	require.False(t, simulation.state.started)
	require.True(t, simulation.Healthy())
}
