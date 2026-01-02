package listener

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
)

// TestExecute confirms that the launcher listener can accept client connections
// and receive data from them.
func TestExecute(t *testing.T) {
	t.Parallel()

	// Set up dependencies
	rootDir := t.TempDir()
	testPrefix := "test"
	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("RootDirectory").Return(rootDir).Maybe()

	// Set up listener
	testListener, err := NewLauncherListener(mockKnapsack, slogger, testPrefix)
	require.NoError(t, err)
	require.NotNil(t, testListener.listener)
	t.Cleanup(func() { testListener.Interrupt(errors.New("test error")) })

	// Start execution
	go testListener.Execute()

	// Find socket
	clientConn, err := NewLauncherClientConnection(rootDir, testPrefix)
	require.NoError(t, err)
	t.Cleanup(func() { clientConn.Close() })

	// Send data
	testData := "test string to send"
	_, err = clientConn.Write([]byte(testData))
	require.NoError(t, err)

	// Wait just a bit for the message to be received
	time.Sleep(3 * time.Second)

	// Confirm that the listener received and logged the string
	logLines := logBytes.String()
	require.Contains(t, logLines, testData)
}

// TestInterrupt_Multiple confirms that Interrupt can be called multiple times without blocking;
// we require this for rungroup actors.
func TestInterrupt_Multiple(t *testing.T) {
	t.Parallel()

	// Set up dependencies
	rootDir := t.TempDir()
	testPrefix := "test"
	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("RootDirectory").Return(rootDir).Maybe()

	// Set up listener
	testListener, err := NewLauncherListener(mockKnapsack, slogger, testPrefix)
	require.NoError(t, err)

	// Start and then interrupt
	go testListener.Execute()
	time.Sleep(3 * time.Second)
	interruptStart := time.Now()
	testListener.Interrupt(errors.New("test error"))

	// Confirm we can call Interrupt multiple times without blocking
	interruptComplete := make(chan struct{})
	expectedInterrupts := 3
	for i := 0; i < expectedInterrupts; i += 1 {
		go func() {
			testListener.Interrupt(nil)
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
			t.Errorf("could not call interrupt multiple times and return within 5 seconds -- interrupted at %s, received %d interrupts before timeout; logs: \n%s\n", interruptStart.String(), receivedInterrupts, logBytes.String())
			t.FailNow()
		}
	}

	require.Equal(t, expectedInterrupts, receivedInterrupts)
}
