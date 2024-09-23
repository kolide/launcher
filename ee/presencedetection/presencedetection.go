package presencedetection

import (
	"fmt"
	"sync"
	"time"
)

type PresenceDetector struct {
	lastDetectionUTC time.Time
	mutext           sync.Mutex
}

func (pd *PresenceDetector) DetectPresence(reason string, detectionInterval time.Duration) (bool, error) {
	pd.mutext.Lock()
	defer pd.mutext.Unlock()

	// Check if the last detection was within the detection interval
	if time.Since(pd.lastDetectionUTC) < detectionInterval {
		return true, nil
	}

	success, err := Detect(reason)
	if err != nil {
		return false, fmt.Errorf("detecting presence: %w", err)
	}

	if success {
		pd.lastDetectionUTC = time.Now().UTC()
	}

	return success, nil
}
