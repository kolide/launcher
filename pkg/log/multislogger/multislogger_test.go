package multislogger

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	"log/slog"

	"github.com/kolide/kit/ulid"
	"github.com/stretchr/testify/require"
)

func TestMultiSlogger(t *testing.T) {
	t.Parallel()

	var systemLogBuf, shipperBuf, debugLogBuf bytes.Buffer

	clearBufsFn := func() {
		systemLogBuf.Reset()
		shipperBuf.Reset()
		debugLogBuf.Reset()
	}

	multislogger := New()
	multislogger.Logger.Debug("don't panic")

	multislogger.AddHandler(slog.NewJSONHandler(&systemLogBuf, &slog.HandlerOptions{Level: slog.LevelInfo}), SystemLogMatcher)
	multislogger.AddHandler(slog.NewJSONHandler(&debugLogBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	shipperLogLevel := new(slog.LevelVar)
	shipperLogLevel.Set(slog.LevelInfo)
	multislogger.AddHandler(slog.NewJSONHandler(&shipperBuf, &slog.HandlerOptions{Level: shipperLogLevel}))

	multislogger.Logger.Debug("debug_msg")

	require.Contains(t, debugLogBuf.String(), "debug_msg", "should be in debug log since it's debug level")
	require.Empty(t, shipperBuf.String(), "should not be in shipper log since it's debug level")
	require.Empty(t, systemLogBuf.String(), "should not be in system log since there was no system_log=true attr")
	clearBufsFn()

	multislogger.Logger.Info("info_msg")

	require.Contains(t, debugLogBuf.String(), "info_msg", "should be in debug log since it's info level and that's higher than debug level")
	require.Contains(t, shipperBuf.String(), "info_msg", "should be in shipper log since it's info level")
	require.Empty(t, systemLogBuf.String(), "should not be in system log since there was no system_log=true attr")
	clearBufsFn()

	multislogger.Logger.Info("system_log_msg", "system_log", true)

	require.Contains(t, debugLogBuf.String(), "system_log_msg", "should be in debug log since it's info level and that's higher than debug level")
	require.Contains(t, shipperBuf.String(), "system_log_msg", "should be in shipper log since it's info level")
	require.Contains(t, systemLogBuf.String(), "system_log_msg", "should be in system log since there was system_log=true attr")
	clearBufsFn()

	// set shipper level to debug
	shipperLogLevel.Set(slog.LevelDebug)
	multislogger.Logger.Debug("debug_msg")

	require.Contains(t, debugLogBuf.String(), "debug_msg", "should be in debug log since it's debug level")
	require.Contains(t, shipperBuf.String(), "debug_msg", "should now be in shipper log since it's level was set to debug")
	require.Empty(t, systemLogBuf.String(), "should not be in system log since there was no system_log=true attr")
	clearBufsFn()

	// ensure that span_id gets added as an attribute when present in context
	spanId := ulid.New()
	ctx := context.WithValue(context.Background(), "span_id", spanId)
	multislogger.Logger.Log(ctx, slog.LevelDebug, "info_with_interesting_ctx_value")

	require.Contains(t, debugLogBuf.String(), "info_with_interesting_ctx_value", "should be in debug log since it's debug level")
	requireContainsAttribute(t, &debugLogBuf, "span_id", spanId)

	require.Contains(t, shipperBuf.String(), "info_with_interesting_ctx_value", "should now be in shipper log since it's new handler was set to debug level")
	requireContainsAttribute(t, &shipperBuf, "span_id", spanId)

	require.Empty(t, systemLogBuf.String(), "should not be in system log since there was no system_log=true attr")
	clearBufsFn()

	// ensure that we are not duplicating values
	spanId = ulid.New()
	ctx = context.WithValue(context.Background(), "span_id", spanId)
	multislogger.Logger.Log(ctx, slog.LevelDebug, "info_with_interesting_ctx_value")

	require.Contains(t, debugLogBuf.String(), "info_with_interesting_ctx_value", "should be in debug log since it's debug level")
	requireContainsAttribute(t, &debugLogBuf, "span_id", spanId)

	require.Contains(t, shipperBuf.String(), "info_with_interesting_ctx_value", "should now be in shipper log since it's new handler was set to debug level")
	requireContainsAttribute(t, &shipperBuf, "span_id", spanId)

	require.Empty(t, systemLogBuf.String(), "should not be in system log since there was no system_log=true attr")
	clearBufsFn()

}

func requireContainsAttribute(t *testing.T, r io.Reader, key, value string) {
	for _, data := range jsonl(t, r) {
		if v, ok := data[key]; ok {
			require.Equal(t, value, v)
			return
		}
	}

	t.Fatal("attribute not found")
}

func jsonl(t *testing.T, reader io.Reader) []map[string]interface{} {
	var result []map[string]interface{}

	decoder := json.NewDecoder(reader)
	for {
		var data map[string]interface{}

		err := decoder.Decode(&data)
		if err == io.EOF {
			break
		}

		require.NoError(t, err)
		result = append(result, data)
	}

	return result
}
