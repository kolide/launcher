package tablewrapper

import (
	"context"
	"testing"
	"time"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/osquery/osquery-go/gen/osquery"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCall(t *testing.T) {
	t.Parallel()

	expectedName := "test_table"
	overrideTimeout := 3 * time.Second
	expectedRow := map[string]string{
		"somekey": "somevalue",
	}
	expectedRows := []map[string]string{
		expectedRow,
	}

	mockFlags := typesmocks.NewFlags(t)
	mockFlags.On("TableGenerateTimeout").Return(4 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, keys.TableGenerateTimeout).Return()

	w := New(mockFlags, multislogger.NewNopLogger(), expectedName, nil, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return expectedRows, nil
	}, WithTableGenerateTimeout(overrideTimeout))

	// The table name must be set correctly
	require.Equal(t, expectedName, w.Name())

	// The generate function must work
	resp := w.Call(t.Context(), map[string]string{"action": "generate", "context": "{}"})
	require.Equal(t, int32(0), resp.Status.Code) // success
	require.Equal(t, 1, len(resp.Response))
	require.Equal(t, expectedRow, resp.Response[0])

	mockFlags.AssertExpectations(t)
}

func TestCall_handlesTimeout(t *testing.T) {
	t.Parallel()

	expectedName := "test_table"
	overrideTimeout := 3 * time.Second

	mockFlags := typesmocks.NewFlags(t)
	mockFlags.On("TableGenerateTimeout").Return(4 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, keys.TableGenerateTimeout).Return()

	w := New(mockFlags, multislogger.NewNopLogger(), expectedName, nil, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		// The generate function must take longer than the timeout
		time.Sleep(3 * overrideTimeout)
		return []map[string]string{
			{
				"somekey": "1",
			},
		}, nil
	}, WithTableGenerateTimeout(overrideTimeout))

	// The table name must be set correctly
	require.Equal(t, expectedName, w.Name())

	// The call to generate must return by the time we hit `overrideTimeout`
	resultChan := make(chan osquery.ExtensionResponse)
	go func() {
		resultChan <- w.Call(t.Context(), map[string]string{"action": "generate", "context": "{}"})
	}()

	select {
	case resp := <-resultChan:
		// We got a result once we hit the timeout
		require.Equal(t, int32(1), resp.Status.Code) // failure
		require.Contains(t, resp.Status.Message, "error generating table")
	case <-time.After(overrideTimeout + 1*time.Second):
		t.Error("generate did not return within timeout")
		t.FailNow()
	}

	mockFlags.AssertExpectations(t)
}

func TestCall_allowsConcurrentRequests(t *testing.T) {
	t.Parallel()

	expectedName := "test_table"
	overrideTimeout := 3 * time.Second
	expectedRow := map[string]string{
		"somekey": "somevalue",
	}
	expectedRows := []map[string]string{
		expectedRow,
	}

	mockFlags := typesmocks.NewFlags(t)
	mockFlags.On("TableGenerateTimeout").Return(4 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, keys.TableGenerateTimeout).Return()

	w := New(mockFlags, multislogger.NewNopLogger(), expectedName, nil, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		time.Sleep(100 * time.Millisecond) // very short wait -- generate will not time out
		return expectedRows, nil
	}, WithTableGenerateTimeout(overrideTimeout))

	// The table name must be set correctly
	require.Equal(t, expectedName, w.Name())

	// Concurrent requests to the table must succeed
	resultChan := make(chan osquery.ExtensionResponse)
	for i := 0; i < numWorkers; i += 1 {
		go func() {
			resultChan <- w.Call(t.Context(), map[string]string{"action": "generate", "context": "{}"})
		}()
	}

	// Make sure we get five successful responses
	for i := 0; i < numWorkers; i += 1 {
		select {
		case resp := <-resultChan:
			require.Equal(t, int32(0), resp.Status.Code, resp.Status.Message) // success
			require.Equal(t, 1, len(resp.Response))
			require.Equal(t, expectedRow, resp.Response[0])
		case <-time.After(overrideTimeout + 1*time.Second):
			t.Error("generate did not return within timeout")
			t.FailNow()
		}
	}

	mockFlags.AssertExpectations(t)
}

func TestCall_limitsExcessiveConcurrentRequests(t *testing.T) {
	t.Parallel()

	expectedName := "test_table"
	overrideTimeout := 4 * time.Second
	expectedRow := map[string]string{
		"somekey": "somevalue",
	}
	expectedRows := []map[string]string{
		expectedRow,
	}

	mockFlags := typesmocks.NewFlags(t)
	mockFlags.On("TableGenerateTimeout").Return(4 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, keys.TableGenerateTimeout).Return()

	w := New(mockFlags, multislogger.NewNopLogger(), expectedName, nil, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		time.Sleep(overrideTimeout + 1*time.Second) // generate should always time out
		return expectedRows, nil
	}, WithTableGenerateTimeout(overrideTimeout))

	// The table name must be set correctly
	require.Equal(t, expectedName, w.Name())

	// The first five requests to the table must should time out. Any subsequent requests
	// that occur while the first five have not yet timed out should immediately return
	// an error.
	gotWorkerChan := make(chan osquery.ExtensionResponse, numWorkers)
	for i := 0; i < numWorkers; i += 1 {
		go func() {
			gotWorkerChan <- w.Call(t.Context(), map[string]string{"action": "generate", "context": "{}"})
		}()
		time.Sleep(100 * time.Millisecond) // small sleep to make it easier to enforce ordering of calls
	}
	noWorkersAvailableChan := make(chan osquery.ExtensionResponse, numWorkers)
	for i := 0; i < numWorkers; i += 1 {
		go func() {
			noWorkersAvailableChan <- w.Call(t.Context(), map[string]string{"action": "generate", "context": "{}"})
		}()
		time.Sleep(100 * time.Millisecond) // small sleep to make it easier to enforce ordering of calls
	}

	// The first five requests to the table should have timed out
	for i := 0; i < numWorkers; i += 1 {
		select {
		case resp := <-gotWorkerChan:
			require.Equal(t, int32(1), resp.Status.Code)                // failure
			require.Contains(t, resp.Status.Message, "timed out after") // matches `querying %s timed out after %s (queried columns: %v)`
		case <-time.After(overrideTimeout + 1*time.Second):
			t.Error("generate did not return within timeout")
			t.FailNow()
		}
	}

	// The next few requests to the table should also not succeed because workers
	// could not be acquired in time -- but we still expect them to return in a timely manner
	for i := 0; i < numWorkers; i += 1 {
		select {
		case resp := <-noWorkersAvailableChan:
			require.Equal(t, int32(1), resp.Status.Code)                     // failure
			require.Contains(t, resp.Status.Message, "no workers available") // matches `no workers available (limit %d)`
		case <-time.After(overrideTimeout + 1*time.Second):
			t.Error("generate did not return within timeout")
			t.FailNow()
		}
	}

	// Sleep to ensure the workers are now all available -- make another request and see it hit the regular timeout
	time.Sleep(overrideTimeout) // 1 second should be sufficient, but we sleep the full override timeout to avoid test flakiness
	resp := w.Call(t.Context(), map[string]string{"action": "generate", "context": "{}"})
	require.Equal(t, int32(1), resp.Status.Code)                // failure
	require.Contains(t, resp.Status.Message, "timed out after") // matches `querying %s timed out after %s (queried columns: %v)`

	mockFlags.AssertExpectations(t)
}

func TestFlagsChanged(t *testing.T) {
	t.Parallel()

	expectedName := "test_table"
	overrideTimeout := 3 * time.Second
	expectedRow := map[string]string{
		"somekey": "somevalue",
	}
	expectedRows := []map[string]string{
		expectedRow,
	}

	mockFlags := typesmocks.NewFlags(t)
	mockFlags.On("TableGenerateTimeout").Return(4 * time.Minute).Once()
	mockFlags.On("RegisterChangeObserver", mock.Anything, keys.TableGenerateTimeout).Return()

	w := newWrappedTable(mockFlags, multislogger.NewNopLogger(), expectedName, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return expectedRows, nil
	}, WithTableGenerateTimeout(overrideTimeout))

	// The table name must be set correctly
	require.Equal(t, expectedName, w.name)

	// The timeout must be set to the override to start
	w.genTimeoutLock.Lock()
	require.Equal(t, overrideTimeout, w.genTimeout)
	w.genTimeoutLock.Unlock()

	// Simulate TableGenerateTimeout changing via control server
	controlServerOverrideTimeout := 30 * time.Second
	mockFlags.On("TableGenerateTimeout").Return(controlServerOverrideTimeout).Once()
	w.FlagsChanged(t.Context(), keys.TableGenerateTimeout)

	// The timeout should have been updated
	w.genTimeoutLock.Lock()
	require.Equal(t, controlServerOverrideTimeout, w.genTimeout)
	w.genTimeoutLock.Unlock()
}
