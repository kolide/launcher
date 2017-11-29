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

func TestLaunchFailsWithNoConfiguration(t *testing.T) {
	simulation := LaunchSimulation()

	// Sleep briefly so that the async startup can fail
	time.Sleep(100 * time.Millisecond)

	// The instance should no be healthy
	require.False(t, simulation.Healthy())
}
