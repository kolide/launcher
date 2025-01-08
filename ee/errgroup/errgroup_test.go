package errgroup

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/kolide/launcher/pkg/log/multislogger"
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

			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			eg := NewLoggedErrgroup(ctx, multislogger.NewNopLogger())

			for i, err := range tt.errs {
				err := err
				eg.StartGoroutine(ctx, strconv.Itoa(i), func() error { return err })
				time.Sleep(500 * time.Millisecond) // try to enforce ordering of goroutines
			}

			// We should get the expected error when we wait for the routines to exit
			require.Equal(t, tt.expectedErr, eg.Wait(), "incorrect error returned by errgroup")

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

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	eg := NewLoggedErrgroup(ctx, multislogger.NewNopLogger())

	eg.StartGoroutine(ctx, "test_goroutine", func() error {
		return nil
	})

	// We should get the expected error when we wait for the routines to exit
	eg.Shutdown()
	require.Nil(t, eg.Wait(), "should not have returned error on shutdown")

	// We expect that the errgroup shuts down
	canceled := false
	select {
	case <-eg.Exited():
		canceled = true
	default:
	}

	require.True(t, canceled, "errgroup did not exit")
}

func Test_HandlesPanic(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	eg := NewLoggedErrgroup(ctx, multislogger.NewNopLogger())

	eg.StartGoroutine(ctx, "test_goroutine", func() error {
		testArr := make([]int, 0)
		fmt.Println(testArr[2]) // cause out-of-bounds panic
		return nil
	})

	// We expect that the errgroup shuts down -- the test should not panic
	eg.Shutdown()
	require.Error(t, eg.Wait(), "should have returned error from panicking goroutine")
	canceled := false
	select {
	case <-eg.Exited():
		canceled = true
	default:
	}

	require.True(t, canceled, "errgroup did not exit")
}
