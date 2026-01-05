package listener

import (
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	"github.com/kolide/launcher/ee/agent/types"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
)

// TestEnroll confirms that the launcher listener can accept client connections
// and receive enrollment requests from them.
func TestEnroll(t *testing.T) {
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
	tokenStore, err := storageci.NewStore(t, slogger, storage.TokenStore.String())
	require.NoError(t, err)
	mockKnapsack.On("TokenStore").Return(tokenStore)

	// Expect that we're initially unenrolled, and then enroll upon processing request
	mockKnapsack.On("CurrentEnrollmentStatus").Return(types.Unenrolled, nil).Once()
	mockKnapsack.On("CurrentEnrollmentStatus").Return(types.Enrolled, nil)

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
	enrollSecret := createTestEnrollSecret(t, "test-munemo")
	require.NoError(t, clientConn.Enroll(enrollSecret))

	// Confirm that the listener received the enrollment request
	logLines := logBytes.String()
	require.Contains(t, logLines, "processing request to enroll")

	// Confirm that the listener stored the enrollment secret
	storedToken, err := tokenStore.Get(storage.KeyByIdentifier(storage.EnrollmentSecretTokenKey, storage.IdentifierTypeRegistration, []byte(types.DefaultRegistrationID)))
	require.NoError(t, err)
	require.Equal(t, enrollSecret, string(storedToken))
}

// TestEnroll_Invalid confirms that the launcher listener will reject
// enrollment requests with invalid JWTs.
func TestEnroll_Invalid(t *testing.T) {
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
	mockKnapsack.On("CurrentEnrollmentStatus").Return(types.Unenrolled, nil)

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
	invalidEnrollSecret := ulid.New()
	require.Error(t, clientConn.Enroll(invalidEnrollSecret))

	emptyEnrollSecret := ""
	require.Error(t, clientConn.Enroll(emptyEnrollSecret))
}

// TestEnroll_Unenrolled confirms that the launcher listener will reject
// enrollment requests when already enrolled.
func TestEnroll_Unenrolled(t *testing.T) {
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
	mockKnapsack.On("CurrentEnrollmentStatus").Return(types.Enrolled, nil)

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
	enrollSecret := createTestEnrollSecret(t, "test-munemo")
	require.Error(t, clientConn.Enroll(enrollSecret))
}

// TestInterrupt_Cleanup confirms that the socket file is cleaned up on interrupt.
func TestInterrupt_Cleanup(t *testing.T) {
	t.Parallel()

	// Set up dependencies
	rootDir := t.TempDir()
	testPrefix := "test"
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("RootDirectory").Return(rootDir).Maybe()

	// Set up listener
	testListener, err := NewLauncherListener(mockKnapsack, multislogger.NewNopLogger(), testPrefix)
	require.NoError(t, err)
	require.NotNil(t, testListener.listener)
	t.Cleanup(func() { testListener.Interrupt(errors.New("test error")) })

	// Start execution and wait a couple seconds
	go testListener.Execute()
	time.Sleep(3 * time.Second)

	// Interrupt
	testListener.Interrupt(errors.New("test error"))

	// Confirm socket path no longer exists
	_, err = os.Stat(testListener.socketPath)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))
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

// createTestEnrollSecret creates a JWT that can be parsed by the extension
// to extract its munemo.
func createTestEnrollSecret(t *testing.T, munemo string) string {
	testSigningKey := []byte("test-key")

	type CustomKolideJwtClaims struct {
		Munemo string `json:"organization"`
		jwt.RegisteredClaims
	}

	claims := CustomKolideJwtClaims{
		munemo,
		jwt.RegisteredClaims{
			// A usual scenario is to set the expiration time relative to the current time
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "test",
			Subject:   "somebody",
			ID:        "1",
			Audience:  []string{"somebody_else"},
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedTokenStr, err := token.SignedString(testSigningKey)
	require.NoError(t, err)

	return signedTokenStr
}
