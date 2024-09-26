package presencedetection

import (
	"fmt"
	"sync"
	"time"
)

const DetectionFailedDurationValue = -1 * time.Second

type PresenceDetector struct {
	lastDetectionUTC          time.Time
	mutext                    sync.Mutex
	hadOneSuccessfulDetection bool
	// detectFunc that can be set for testing
	detectFunc func(string) (bool, error)
}

// DetectPresence checks if the user is present by detecting the presence of a user.
// It returns the duration since the last detection.
func (pd *PresenceDetector) DetectPresence(reason string, detectionInterval time.Duration) (time.Duration, error) {
	pd.mutext.Lock()
	defer pd.mutext.Unlock()

	if pd.detectFunc == nil {
		pd.detectFunc = Detect
	}

	// Check if the last detection was within the detection interval
	if pd.hadOneSuccessfulDetection && time.Since(pd.lastDetectionUTC) < detectionInterval {
		return time.Since(pd.lastDetectionUTC), nil
	}

	success, err := pd.detectFunc(reason)

	switch {
	case err != nil && pd.hadOneSuccessfulDetection:
		return time.Since(pd.lastDetectionUTC), fmt.Errorf("detecting presence: %w", err)

	case err != nil: // error without initial successful detection
		return DetectionFailedDurationValue, fmt.Errorf("detecting presence: %w", err)

	case success:
		pd.lastDetectionUTC = time.Now().UTC()
		pd.hadOneSuccessfulDetection = true
		return 0, nil

	default: // failed detection without error, maybe not possible?
		return time.Since(pd.lastDetectionUTC), fmt.Errorf("detection failed without error")
	}
}
