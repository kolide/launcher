package presencedetection

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPresenceDetector_DetectPresence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                       string
		interval                   time.Duration
		detectFunc                 func(string) (bool, error)
		initialLastDetectionUTC    time.Time
		hadOneSuccessfulDetection  bool
		errAssertion               assert.ErrorAssertionFunc
		expectedLastDetectionDelta time.Duration
	}{
		{
			name: "first detection success",
			detectFunc: func(string) (bool, error) {
				return true, nil
			},
			errAssertion:               assert.NoError,
			expectedLastDetectionDelta: 0,
		},
		{
			name: "detection within interval",
			detectFunc: func(string) (bool, error) {
				return false, errors.New("should not have called detectFunc, since within interval")
			},
			errAssertion:              assert.NoError,
			initialLastDetectionUTC:   time.Now().UTC(),
			interval:                  time.Minute,
			hadOneSuccessfulDetection: true,
		},
		{
			name: "error first detection",
			detectFunc: func(string) (bool, error) {
				return false, errors.New("error")
			},
			errAssertion:               assert.Error,
			expectedLastDetectionDelta: -1,
		},
		{
			name: "error after first detection",
			detectFunc: func(string) (bool, error) {
				return false, errors.New("error")
			},
			errAssertion:              assert.Error,
			initialLastDetectionUTC:   time.Now().UTC(),
			hadOneSuccessfulDetection: true,
		},
		{
			name: "detection failed without OS error",
			detectFunc: func(string) (bool, error) {
				return false, nil
			},
			errAssertion:              assert.Error,
			initialLastDetectionUTC:   time.Now().UTC(),
			hadOneSuccessfulDetection: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pd := &PresenceDetector{
				detectFunc:                tt.detectFunc,
				lastDetectionUTC:          tt.initialLastDetectionUTC,
				hadOneSuccessfulDetection: tt.hadOneSuccessfulDetection,
			}

			timeSinceLastDetection, err := pd.DetectPresence("this is a test", tt.interval)
			tt.errAssertion(t, err)

			delta := timeSinceLastDetection - tt.expectedLastDetectionDelta
			assert.LessOrEqual(t, delta, time.Second)
		})
	}
}
