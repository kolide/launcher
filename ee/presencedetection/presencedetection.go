package presencedetection

import (
	"fmt"
	"sync"
	"time"
)

const (
	DetectionFailedDurationValue        = -1 * time.Second
	DetectionTimeout                    = 1 * time.Minute
	DefaultMinDetectionAttmemptInterval = 3 * time.Second
)

type PresenceDetector struct {
	lastDetection time.Time
	mutex         sync.Mutex
	// detector is an interface to allow for mocking in tests
	detector                    detectorIface
	lastDetectionAttempt        time.Time
	minDetectionAttemptInterval time.Duration
}

// just exists for testing purposes
type detectorIface interface {
	Detect(reason string) (bool, error)
}

type detector struct{}

func (d *detector) Detect(reason string) (bool, error) {
	return Detect(reason)
}

// DetectPresence checks if the user is present by detecting the presence of a user.
// It returns the duration since the last detection.
func (pd *PresenceDetector) DetectPresence(reason string, detectionInterval time.Duration) (time.Duration, error) {
	pd.mutex.Lock()
	defer pd.mutex.Unlock()

	if pd.detector == nil {
		pd.detector = &detector{}
	}

	// Check if the last detection was within the detection interval
	if (pd.lastDetection != time.Time{}) && time.Since(pd.lastDetection) < detectionInterval {
		return time.Since(pd.lastDetection), nil
	}

	minDetetionInterval := pd.minDetectionAttemptInterval
	if minDetetionInterval == 0 {
		minDetetionInterval = DefaultMinDetectionAttmemptInterval
	}

	// if the user fails or cancels the presence detection, we want to wait a bit before trying again
	// so that if there are several queued up requests, we don't prompt the user multiple times in a row
	// if they keep hitting cancel
	if time.Since(pd.lastDetectionAttempt) < minDetetionInterval {
		return time.Since(pd.lastDetection), nil
	}

	success, err := pd.detector.Detect(reason)
	pd.lastDetectionAttempt = time.Now()
	if err != nil {
		// if we got an error, we behave as if there have been no successful detections in the past
		return DetectionFailedDurationValue, fmt.Errorf("detecting presence: %w", err)
	}

	if success {
		pd.lastDetection = time.Now().UTC()
		return 0, nil
	}

	// if we got here it means we failed without an error
	// this "should" never happen, but here for completeness
	return DetectionFailedDurationValue, fmt.Errorf("detection failed without OS error")
}
