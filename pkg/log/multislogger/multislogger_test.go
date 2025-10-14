package multislogger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"log/slog"

	"github.com/kolide/kit/ulid"
	"github.com/stretchr/testify/require"
)

func TestMultiSlogger(t *testing.T) {
	t.Parallel()

	var shipperBuf, debugLogBuf bytes.Buffer

	clearBufsFn := func() {
		shipperBuf.Reset()
		debugLogBuf.Reset()
	}

	multislogger := New()
	multislogger.Logger.DebugContext(t.Context(), "dont panic")

	multislogger = New(slog.NewJSONHandler(&debugLogBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	shipperLogLevel := new(slog.LevelVar)
	shipperLogLevel.Set(slog.LevelInfo)
	multislogger.AddHandler(slog.NewJSONHandler(&shipperBuf, &slog.HandlerOptions{Level: shipperLogLevel}))

	multislogger.Logger.DebugContext(t.Context(), "debug_msg")

	require.Contains(t, debugLogBuf.String(), "debug_msg", "should be in debug log since it's debug level")
	require.Empty(t, shipperBuf.String(), "should not be in shipper log since it's debug level")
	clearBufsFn()

	multislogger.Logger.InfoContext(t.Context(), "info_msg")

	require.Contains(t, debugLogBuf.String(), "info_msg", "should be in debug log since it's info level and that's higher than debug level")
	require.Contains(t, shipperBuf.String(), "info_msg", "should be in shipper log since it's info level")
	clearBufsFn()

	// set shipper level to debug
	shipperLogLevel.Set(slog.LevelDebug)
	multislogger.Logger.DebugContext(t.Context(), "debug_msg_2")

	require.Contains(t, debugLogBuf.String(), "debug_msg_2", "should be in debug log since it's debug level")
	require.Contains(t, shipperBuf.String(), "debug_msg_2", "should now be in shipper log since it's level was set to debug")
	clearBufsFn()

	// ensure that span_id gets added as an attribute when present in context
	spanId := ulid.New()
	ctx := context.WithValue(t.Context(), SpanIdKey, spanId)
	multislogger.Logger.Log(ctx, slog.LevelDebug, "info_with_interesting_ctx_value")

	require.Contains(t, debugLogBuf.String(), "info_with_interesting_ctx_value", "should be in debug log since it's debug level")
	requireContainsAttribute(t, &debugLogBuf, SpanIdKey.String(), spanId)

	require.Contains(t, shipperBuf.String(), "info_with_interesting_ctx_value", "should now be in shipper log since it's new handler was set to debug level")
	requireContainsAttribute(t, &shipperBuf, SpanIdKey.String(), spanId)
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

func TestInterrupt_Multiple(t *testing.T) {
	t.Parallel()

	var logBuf bytes.Buffer
	multislogger := New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	ctx := t.Context()
	executeFunc := multislogger.ExecuteWithContext(ctx)

	// Start and then interrupt
	go executeFunc()
	time.Sleep(100 * time.Millisecond) // Give it time to start
	interruptStart := time.Now()
	multislogger.Interrupt(errors.New("test error"))

	// Confirm we can call Interrupt multiple times without blocking
	interruptComplete := make(chan struct{})
	expectedInterrupts := 3
	for i := 0; i < expectedInterrupts; i += 1 {
		go func() {
			multislogger.Interrupt(nil)
			interruptComplete <- struct{}{}
		}()
	}

	receivedInterrupts := 0
	for {
		if receivedInterrupts >= expectedInterrupts {
			break
		}

		select {
		case <-interruptComplete:
			receivedInterrupts += 1
			continue
		case <-time.After(5 * time.Second):
			t.Errorf("could not call interrupt multiple times and return within 5 seconds -- interrupted at %s, received %d interrupts before timeout; logs: \n%s\n", interruptStart.String(), receivedInterrupts, logBuf.String())
			t.FailNow()
		}
	}

	require.Equal(t, expectedInterrupts, receivedInterrupts)
}
