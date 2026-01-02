package listener

import (
	"errors"
	"log/slog"
	"net"
	"testing"
	"time"

	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
)

// TestNewLauncherListener confirms that NewLauncherListener correctly sets up a net listener.
func TestNewLauncherListener(t *testing.T) {
	t.Parallel()

	// Set up dependencies
	rootDir := t.TempDir()
	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("RootDirectory").Return(rootDir).Maybe()

	// Set up listener
	testListener, err := NewLauncherListener(mockKnapsack, slogger, "test")
	require.NoError(t, err)
	require.NotNil(t, testListener.listener)
	t.Cleanup(func() { testListener.Interrupt(errors.New("test")) })

	// Confirm pipe works: can create a client connection
	clientConn, err := net.Dial("unix", testListener.listener.Addr().String())
	require.NoError(t, err)
	t.Cleanup(func() { clientConn.Close() })
	serverConn, err := testListener.listener.Accept()
	require.NoError(t, err)
	t.Cleanup(func() { serverConn.Close() })

	// Confirm pipe works: can send and read data over the connection
	testData := []byte("test string to send")
	_, err = clientConn.Write(testData)
	require.NoError(t, err)

	testBuffer := make([]byte, len(testData))
	_, err = serverConn.Read(testBuffer)
	require.NoError(t, err)
	require.Equal(t, testData, testBuffer)
}

// TestInterrupt_Multiple confirms that Interrupt can be called multiple times without blocking;
// we require this for rungroup actors.
func TestInterrupt_Multiple(t *testing.T) {
	t.Parallel()

	// Set up dependencies
	rootDir := t.TempDir()
	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("RootDirectory").Return(rootDir).Maybe()

	// Set up listener
	testListener, err := NewLauncherListener(mockKnapsack, slogger, "test")
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
