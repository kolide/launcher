package presencedetection

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/kolide/launcher/ee/presencedetection/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestPresenceDetector_DetectPresence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                    string
		interval                time.Duration
		detector                func(t *testing.T) detectorIface
		initialLastDetectionUTC time.Time
		expectError             bool
	}{
		{
			name:     "first detection success",
			interval: 0,
			detector: func(t *testing.T) detectorIface {
				d := mocks.NewDetectorIface(t)
				d.On("Detect", mock.AnythingOfType("string"), DetectionTimeout).Return(true, nil)
				return d
			},
		},
		{
			name:     "detection outside interval",
			interval: time.Minute,
			detector: func(t *testing.T) detectorIface {
				d := mocks.NewDetectorIface(t)
				d.On("Detect", mock.AnythingOfType("string"), DetectionTimeout).Return(true, nil)
				return d
			},
			initialLastDetectionUTC: time.Now().UTC().Add(-time.Minute),
		},
		{
			name:     "detection within interval",
			interval: time.Minute,
			detector: func(t *testing.T) detectorIface {
				// should not be called, will get error if it is
				return mocks.NewDetectorIface(t)
			},
			initialLastDetectionUTC: time.Now().UTC(),
		},
		{
			name:     "error first detection",
			interval: 0,
			detector: func(t *testing.T) detectorIface {
				d := mocks.NewDetectorIface(t)
				d.On("Detect", mock.AnythingOfType("string"), DetectionTimeout).Return(true, errors.New("error"))
				return d
			},
			expectError: true,
		},
		{
			name:     "error after first detection",
			interval: 0,
			detector: func(t *testing.T) detectorIface {
				d := mocks.NewDetectorIface(t)
				d.On("Detect", mock.AnythingOfType("string"), DetectionTimeout).Return(true, errors.New("error"))
				return d
			},
			initialLastDetectionUTC: time.Now().UTC(),
			expectError:             true,
		},
		{
			// this should never happen, but it is here for completeness
			name:     "detection failed without OS error",
			interval: 0,
			detector: func(t *testing.T) detectorIface {
				d := mocks.NewDetectorIface(t)
				d.On("Detect", mock.AnythingOfType("string"), DetectionTimeout).Return(false, nil)
				return d
			},
			initialLastDetectionUTC: time.Now().UTC(),
			expectError:             true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pd := &PresenceDetector{
				detector:      tt.detector(t),
				lastDetection: tt.initialLastDetectionUTC,
			}

			timeSinceLastDetection, err := pd.DetectPresence("this is a test", tt.interval)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			absDelta := math.Abs(timeSinceLastDetection.Seconds() - tt.interval.Seconds())
			assert.LessOrEqual(t, absDelta, tt.interval.Seconds())
		})
	}
}
