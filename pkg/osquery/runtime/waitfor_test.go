package runtime

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWaitFor(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name               string
		innerFn            func() error
		errorAssertion     require.ErrorAssertionFunc
		testifyExpectation func(require.TestingT, func() bool, time.Duration, time.Duration, ...interface{})
		errorContains      []string

		interval time.Duration
		timeout  time.Duration
	}{
		{
			name:               "never returns",
			innerFn:            innerFuncGenerator(30*time.Millisecond, nil),
			errorAssertion:     require.NoError,
			testifyExpectation: require.Never,
			interval:           2 * time.Millisecond,
			timeout:            5 * time.Millisecond,
		},
		{
			name:               "fast returns",
			innerFn:            innerFuncGenerator(1*time.Millisecond, nil),
			errorAssertion:     require.NoError,
			testifyExpectation: require.Eventually,
			interval:           2 * time.Millisecond,
			timeout:            5 * time.Millisecond,
		},
		{
			name:               "fast errors",
			innerFn:            innerFuncGenerator(1*time.Millisecond, errors.New("sentinal")),
			errorAssertion:     require.Error,
			testifyExpectation: require.Eventually,
			errorContains:      []string{"sentinal", "timeout", "3 attempts"},
			interval:           2 * time.Millisecond,
			timeout:            5 * time.Millisecond,
		},
		{
			name:               "slow errors",
			innerFn:            innerFuncGenerator(9*time.Millisecond, errors.New("sentinal")),
			errorAssertion:     require.Error,
			testifyExpectation: require.Eventually,
			errorContains:      []string{"sentinal", "timeout", "2 attempts"},
			interval:           4 * time.Millisecond,
			timeout:            10 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Construct a test function, and a suitable
			// comparison function for require.Never / require.Eventually
			fn := func() bool {
				err := waitFor(tt.innerFn, tt.timeout, tt.interval)
				tt.errorAssertion(t, err)

				for _, s := range tt.errorContains {
					assert.ErrorContains(t, err, s)
				}

				// This return is what causes Never or Eventual to Succeed or Fail.
				return true
			}

			tt.testifyExpectation(t, fn, 30*time.Millisecond, 2*time.Millisecond)
		})
	}
}

func innerFuncGenerator(t time.Duration, err error) func() error {
	return func() error {
		time.Sleep(t)
		return err
	}
}
