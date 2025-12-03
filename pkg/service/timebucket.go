package service

import "time"

// timebucket is takes a time.Duration and returns a lightly bucketized one. It's
// purpose is to simplify the logged `took` times so that the log deduplication
// works better.
func timebucket(d time.Duration) time.Duration {
	seconds := int64(d / time.Second)

	// For <= 1s, return 1s
	if seconds <= 1 {
		return 1 * time.Second
	}

	// For <= 2s, return 2s
	if seconds <= 2 {
		return 2 * time.Second
	}

	// For <= 3s, return 3s
	if seconds <= 3 {
		return 3 * time.Second
	}

	// For <= 4s, return 4s
	if seconds <= 4 {
		return 4 * time.Second
	}

	// Group into 3s chunks and return the middle one
	// 5-7 -> 6, 8-10 -> 9, 11-13 -> 12, etc.
	if seconds <= 60 {
		chunk := (seconds - 5) / 3
		middle := 6 + chunk*3
		return time.Duration(middle) * time.Second
	}

	// After 1 minute, return approximate number of minutes
	minutes := seconds / 60
	// Round to nearest minute
	if seconds%60 >= 30 {
		minutes++
	}
	return time.Duration(minutes) * time.Minute
}
