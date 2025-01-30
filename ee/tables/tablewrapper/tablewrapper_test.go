package tablewrapper

import (
	"context"
	"testing"
	"time"

	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/osquery/osquery-go/gen/osquery"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Parallel()

	expectedName := "test_table"
	overrideTimeout := 3 * time.Second
	expectedRow := map[string]string{
		"somekey": "somevalue",
	}
	expectedRows := []map[string]string{
		expectedRow,
	}

	w := New(multislogger.NewNopLogger(), expectedName, nil, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return expectedRows, nil
	}, WithGenerateTimeout(overrideTimeout))

	// The table name must be set correctly
	require.Equal(t, expectedName, w.Name())

	// The generate function must work
	resp := w.Call(context.TODO(), map[string]string{"action": "generate", "context": "{}"})
	require.Equal(t, int32(0), resp.Status.Code) // success
	require.Equal(t, 1, len(resp.Response))
	require.Equal(t, expectedRow, resp.Response[0])
}

func TestNew_handlesTimeout(t *testing.T) {
	t.Parallel()

	expectedName := "test_table"
	overrideTimeout := 3 * time.Second

	w := New(multislogger.NewNopLogger(), expectedName, nil, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		// The generate function must take longer than the timeout
		time.Sleep(3 * overrideTimeout)
		return []map[string]string{
			{
				"somekey": "1",
			},
		}, nil
	}, WithGenerateTimeout(overrideTimeout))

	// The table name must be set correctly
	require.Equal(t, expectedName, w.Name())

	// The call to generate must return by the time we hit `overrideTimeout`
	resultChan := make(chan osquery.ExtensionResponse)
	go func() {
		resultChan <- w.Call(context.TODO(), map[string]string{"action": "generate", "context": "{}"})
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
}
