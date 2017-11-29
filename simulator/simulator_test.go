package simulator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type mockQueryRunner struct{}

func (h *mockQueryRunner) RunQuery(sql string) (results []map[string]string, err error) {
	return []map[string]string{}, nil
}

func TestFunctionalOptions(t *testing.T) {
	simulation := createSimulationRuntime(
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

func TestShutdownSimulation(t *testing.T) {
	simulation := LaunchSimulation(
		WithQueryRunner(&mockQueryRunner{}),
	)

	// Sleep briefly so that everything can start up
	time.Sleep(100 * time.Millisecond)

	require.True(t, simulation.Healthy())
	require.NoError(t, simulation.Shutdown())
}
