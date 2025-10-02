package errgroup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"testing"
	"time"

	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
)

func TestWait(t *testing.T) {
	t.Parallel()

	err1 := errors.New("errgroup_test: 1")
	err2 := errors.New("errgroup_test: 2")

	for _, tt := range []struct {
		testCaseName string
		errs         []error
		expectedErr  error
	}{
		{
			testCaseName: "no error on exit",
			errs:         []error{nil},
			expectedErr:  nil,
		},
		{
			testCaseName: "only first routine has error on exit",
			errs:         []error{err1, nil},
			expectedErr:  err1,
		},
		{
			testCaseName: "only second routine has error on exit",
			errs:         []error{nil, err2},
			expectedErr:  err2,
		},
		{
			testCaseName: "multiple routines have error on exit",
			errs:         []error{err1, nil, err2},
			expectedErr:  err1,
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			eg := NewLoggedErrgroup(ctx, multislogger.NewNopLogger())

			for i, err := range tt.errs {
				err := err
				eg.StartGoroutine(ctx, strconv.Itoa(i), func() error { return err })
				time.Sleep(500 * time.Millisecond) // try to enforce ordering of goroutines
			}

			// We should get the expected error when we wait for the routines to exit
			require.Equal(t, tt.expectedErr, eg.Wait(ctx), "incorrect error returned by errgroup")

			// We expect that the errgroup shuts down
			canceled := false
			select {
			case <-eg.Exited():
				canceled = true
			default:
			}

			require.True(t, canceled, "errgroup did not exit")
		})
	}
}

func TestShutdown(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	eg := NewLoggedErrgroup(ctx, multislogger.NewNopLogger())

	eg.StartGoroutine(ctx, "test_goroutine", func() error {
		return nil
	})

	// We should get the expected error when we wait for the routines to exit
	eg.Shutdown()
	require.Nil(t, eg.Wait(ctx), "should not have returned error on shutdown")

	// We expect that the errgroup shuts down
	canceled := false
	select {
	case <-eg.Exited():
		canceled = true
	default:
	}

	require.True(t, canceled, "errgroup did not exit")
}

func TestShutdown_ReturnsOnTimeout(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}))

	eg := NewLoggedErrgroup(ctx, slogger)

	// Create a goroutine that will not return before the shutdown timeout
	eg.StartGoroutine(ctx, "test_goroutine", func() error {
		time.Sleep(10 * maxErrgroupShutdownDuration)
		return nil
	})

	// Shutdown should return by `maxErrgroupShutdownDuration`
	eg.Shutdown()
	waitChan := make(chan error)
	go func() {
		waitChan <- eg.Wait(ctx)
	}()
	select {
	case err := <-waitChan:
		require.Nil(t, err, "should not have received error")
		require.Contains(t, logBytes.String(), "errgroup did not complete shutdown within timeout")
	case <-time.After(maxErrgroupShutdownDuration + 1*time.Second):
		t.Errorf("instance did not complete shutdown before timeout plus small grace period")
	}

	// We expect that the errgroup shuts down
	canceled := false
	select {
	case <-eg.Exited():
		canceled = true
	default:
	}

	require.True(t, canceled, "errgroup did not exit")
}

func TestStartGoroutine_HandlesPanic(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	eg := NewLoggedErrgroup(ctx, multislogger.NewNopLogger())

	eg.StartGoroutine(ctx, "test_goroutine", func() error {
		testArr := make([]int, 0)
		fmt.Println(testArr[2]) // cause out-of-bounds panic
		return nil
	})

	// We expect that the errgroup shuts itself down -- the test should not panic
	require.Error(t, eg.Wait(ctx), "should have returned error from panicking goroutine")
	canceled := false
	select {
	case <-eg.Exited():
		canceled = true
	default:
	}

	require.True(t, canceled, "errgroup did not exit")
}

func TestStartRepeatedGoroutine_HandlesPanic(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	eg := NewLoggedErrgroup(ctx, multislogger.NewNopLogger())

	eg.StartRepeatedGoroutine(ctx, "test_goroutine", 100*time.Millisecond, 50*time.Millisecond, func() error {
		testArr := make([]int, 0)
		fmt.Println(testArr[2]) // cause out-of-bounds panic
		return nil
	})

	// Wait for long enough that the repeated goroutine executes at least once
	time.Sleep(500 * time.Millisecond)

	// We expect that the errgroup shuts itself down -- the test should not panic
	require.Error(t, eg.Wait(ctx), "should have returned error from panicking goroutine")
	canceled := false
	select {
	case <-eg.Exited():
		canceled = true
	default:
	}

	require.True(t, canceled, "errgroup did not exit")
}

func TestAddShutdownGoroutine_HandlesPanic(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	eg := NewLoggedErrgroup(ctx, multislogger.NewNopLogger())

	eg.AddShutdownGoroutine(ctx, "test_goroutine", func() error {
		testArr := make([]int, 0)
		fmt.Println(testArr[2]) // cause out-of-bounds panic
		return nil
	})

	// Call shutdown so the shutdown goroutine runs and the errgroup returns.
	eg.Shutdown()

	// We expect that the errgroup shuts itself down -- the test should not panic.
	// Since we called `Shutdown`, `Wait` should not return an error.
	require.Nil(t, eg.Wait(ctx), "should not returned error after call to Shutdown")
	canceled := false
	select {
	case <-eg.Exited():
		canceled = true
	default:
	}

	require.True(t, canceled, "errgroup did not exit")
}
