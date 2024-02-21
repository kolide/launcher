package osquery

import (
	"container/list"
)

const (
	// minBytesPerBatch sets the minimum batch size to 0.5mb as lower bound for correction
	minBytesPerBatch = 524288
	// batchIncrementAmount (0.5mb) is the incremental increase amount for the target batch
	// size when previous runs have been successful
	batchIncrementAmount = 524288

	// batchHistoryLen is the number of logPublicationBatches to retain in our publishedBatches linked list
	batchHistoryLen = 15
)

// logPublicationState holds stateful logic to influence the log batch publication size
// depending on prior successes or failures. The primary intent here is to prevent repeatedly
// consuming the entire network bandwidth available to devices that are unable to ship
// the standard maxBytesPerBatch inside of the connection timeout (currently 30 seconds, enforced cloud side)
type (
	logPublicationState struct {
		// maxBytesPerBatch is passed in from the extension opts and respected
		// as a fixed upper limit for batch size, regardless of publication success/failure rates
		maxBytesPerBatch        int
		currentMaxBytesPerBatch int
		publishedBatches        *list.List
	}

	logPublicationBatch struct {
		batchSizeBytes int
		timedOut       bool
	}
)

func NewLogPublicationState(maxBytesPerBatch int) *logPublicationState {
	return &logPublicationState{
		maxBytesPerBatch:        maxBytesPerBatch,
		currentMaxBytesPerBatch: maxBytesPerBatch,
		publishedBatches:        list.New(),
	}
}

func (lps *logPublicationState) exceedsCurrentBatchThreshold(amountBytes int) bool {
	return amountBytes > lps.currentMaxBytesPerBatch
}

func (lps *logPublicationState) recordBatchSuccess(logs []string) {
	lps.recordBatch(logs, false)
	lps.increaseBatchThreshold()
}

func (lps *logPublicationState) recordBatchTimedOut(logs []string) {
	lps.recordBatch(logs, true)
	lps.reduceBatchThreshold()
}

func (lps *logPublicationState) recordBatch(logs []string, timedOut bool) {
	newLogBatch := &logPublicationBatch{
		batchSizeBytes: logBatchSize(logs),
		timedOut: timedOut,
	}

	lps.publishedBatches.PushBack(newLogBatch)
	if lps.publishedBatches.Len() > batchHistoryLen {
		lps.publishedBatches.Remove(lps.publishedBatches.Front())
	}
}

func (lps *logPublicationState) reduceBatchThreshold() {
	if lps.currentMaxBytesPerBatch <= minBytesPerBatch {
		return
	}

	newTargetThreshold := lps.currentMaxBytesPerBatch / 2
	lps.currentMaxBytesPerBatch = maxInt(newTargetThreshold, minBytesPerBatch)
}

func (lps *logPublicationState) increaseBatchThreshold() {
	if lps.currentMaxBytesPerBatch >= lps.maxBytesPerBatch {
		return
	}

	newTargetThreshold := lps.currentMaxBytesPerBatch * 2
	lps.currentMaxBytesPerBatch = minInt(newTargetThreshold, lps.maxBytesPerBatch)
}

// logBatchSize determines the total batch size attempted given the published log batch
func logBatchSize(logs []string) int {
	totalBatchSize := 0
	for _, log := range logs {
		totalBatchSize += len(log)
	}

	return totalBatchSize
}
