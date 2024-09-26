package presencedetection

import (
	"fmt"
	"sync"
	"time"
)

const DetectionFailedDurationValue = -1 * time.Second

type PresenceDetector struct {
	lastDetectionUTC time.Time
	mutext           sync.Mutex
	// detector is an interface to allow for mocking in tests
	detector detectorIface
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
	pd.mutext.Lock()
	defer pd.mutext.Unlock()

	if pd.detector == nil {
		pd.detector = &detector{}
	}

	hadHadSuccessfulDetection := pd.lastDetectionUTC != time.Time{}

	// Check if the last detection was within the detection interval
	if hadHadSuccessfulDetection && time.Since(pd.lastDetectionUTC) < detectionInterval {
		return time.Since(pd.lastDetectionUTC), nil
	}

	success, err := pd.detector.Detect(reason)

	switch {
	case err != nil && hadHadSuccessfulDetection:
		return time.Since(pd.lastDetectionUTC), fmt.Errorf("detecting presence: %w", err)

	case err != nil: // error without initial successful detection
		return DetectionFailedDurationValue, fmt.Errorf("detecting presence: %w", err)

	case success:
		pd.lastDetectionUTC = time.Now().UTC()
		return 0, nil

	default: // failed detection without error, maybe not possible?
		return time.Since(pd.lastDetectionUTC), fmt.Errorf("detection failed without error")
	}
}
