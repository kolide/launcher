package runtime

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kolide/launcher/v2/ee/agent/types"
	"github.com/kolide/launcher/v2/pkg/backoff"
	"github.com/kolide/launcher/v2/pkg/osquery/runtime/history"
	"github.com/kolide/launcher/v2/pkg/threadsafebuffer"
	"github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/require"
)

// waitShutdown shuts the runner down, failing the test if that does not complete within a
// minute. Never retry it: a second Shutdown returns without waiting on the first.
func waitShutdown(t *testing.T, runner *Runner, logBytes *threadsafebuffer.ThreadSafeBuffer) {
	shutdownDone := make(chan struct{})
	go func() {
		runner.Shutdown()
		close(shutdownDone)
	}()

	select {
	case <-shutdownDone:
	case <-time.After(1 * time.Minute):
		t.Error("runner did not shut down within timeout", fmt.Sprintf("runner logs: %s", logBytes.String()))
		t.FailNow()
	}
}

// ensureShutdownOnCleanup shuts down any runner the test did not shut down itself. Failures
// are logged rather than failed, so they don't pile onto whatever already went wrong. This is
// expected to be a no-op on happy paths, which end in waitShutdown instead.
func ensureShutdownOnCleanup(t *testing.T, runner *Runner, logBytes *threadsafebuffer.ThreadSafeBuffer) {
	t.Cleanup(func() {
		if runner.state.Load() == runnerShutdown {
			return
		}

		shutdownDone := make(chan struct{})
		go func() {
			runner.Shutdown()
			close(shutdownDone)
		}()

		select {
		case <-shutdownDone:
		case <-time.After(1 * time.Minute):
			t.Logf("runner did not shut down within timeout. runner logs: %s", logBytes.String())
		}
	})
}

// instancePid returns the pid of the osqueryd process the instance launched.
func instancePid(t *testing.T, runner *Runner, enrollmentId string) int {
	runner.instanceLock.Lock()
	instance, ok := runner.instances[enrollmentId]
	runner.instanceLock.Unlock()
	require.True(t, ok, "no instance for enrollment id", enrollmentId)

	pid, err := instance.pid()
	require.NoError(t, err, "getting osqueryd pid")

	return pid
}

func instanceRunId(runner *Runner, enrollmentId string) string {
	runner.instanceLock.Lock()
	defer runner.instanceLock.Unlock()

	if instance, ok := runner.instances[enrollmentId]; ok {
		return instance.runId
	}
	return ""
}

// waitHealthy waits for every instance to pass the runner's healthcheck.
func waitHealthy(t *testing.T, runner *Runner, logBytes *threadsafebuffer.ThreadSafeBuffer) {
	require.NoError(t,
		backoff.WaitFor(runner.Healthy, osqueryStartupTimeout+socketOpenTimeout, 50*time.Millisecond),
		"instance not healthy before timeout: runner logs:\n\n", logBytes.String())
}

// waitHealthyReplacement waits for the default instance to be replaced and
// healthy, returning the new runId.
func waitHealthyReplacement(t *testing.T, runner *Runner, previousRunId string, logBytes *threadsafebuffer.ThreadSafeBuffer) string {
	require.NoError(t, backoff.WaitFor(func() error {
		if instanceRunId(runner, types.DefaultEnrollmentID) == previousRunId {
			return errors.New("instance not replaced yet")
		}
		return runner.Healthy()
	}, launchRetryDelay+osqueryStartupTimeout+socketOpenTimeout, 50*time.Millisecond),
		"instance not replaced and healthy before timeout: runner logs:\n\n", logBytes.String())

	return instanceRunId(runner, types.DefaultEnrollmentID)
}

// waitConnected waits for connect_time in the latest history entry, which is
// recorded shortly after the instance first becomes healthy.
func waitConnected(t *testing.T, osqHistory *history.History, enrollmentId string) {
	require.NoError(t, backoff.WaitFor(func() error {
		stats, err := osqHistory.LatestInstanceStats(enrollmentId)
		if err != nil {
			return err
		}
		if connectTime, ok := stats["connect_time"]; !ok || connectTime == "" {
			return errors.New("no connect time set for latest instance stats")
		}
		return nil
	}, osqueryStartupTimeout, 50*time.Millisecond), "connect_time never recorded for", enrollmentId)
}

func requireProcessGone(t *testing.T, pid int) {
	exists, err := process.PidExists(int32(pid))
	require.NoError(t, err)
	require.False(t, exists, "osqueryd process still running", pid)
}
