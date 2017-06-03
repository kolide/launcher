package osquery

import (
	"testing"
)

func TestDetectPlatform(t *testing.T) {
	_, err := DetectPlatform()
	if err != nil {
		t.Error("Could not detect platform:", err)
	}
}
