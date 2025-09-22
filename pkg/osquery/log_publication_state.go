package osquery

import "time"

const (
	// minBytesPerBatch sets the minimum batch size to 0.5mb as lower bound for correction
	minBytesPerBatch int = 524288
	// batchIncrementAmount (0.5mb) is the incremental increase amount for the target batch
	// size when previous runs have been successful
	batchIncrementAmount int = 524288
	// maxPublicationDuration is the total time a batch can take without an error triggering a reduction
	// in the max batch size
	maxPublicationDuration time.Duration = 20 * time.Second
)

// logPublicationState holds stateful logic to influence the log batch publication size
// depending on prior successes or failures. The primary intent here is to prevent repeatedly
// consuming the entire network bandwidth available to devices that are unable to ship
// the standard maxBytesPerBatch inside of the connection timeout (currently 30 seconds, enforced cloud side).
// Note that we always expect these batches to be sent sequentially, BeginBatch -> EndBatch
// this would need rework (likely state locking and batch ID tracking) to support concurrent batch publications
type logPublicationState struct {
	// maxBytesPerBatch is passed in from the extension opts and respected
	// as a fixed upper limit for batch size, regardless of publication success/failure rates
	maxBytesPerBatch int
	// currentMaxBytesPerBatch represents the (stateful) upper limit being enforced
	currentMaxBytesPerBatch int
	// currentBatchBufferFilled is used to indicate when a batch's success can be used
	// to increase the threshold (we only want to increase the threshold after sending full, not partial, batches successfully)
	currentBatchBufferFilled bool
	currentBatchStartTime    time.Time
}

func NewLogPublicationState(maxBytesPerBatch int) *logPublicationState {
	return &logPublicationState{
		maxBytesPerBatch:        maxBytesPerBatch,
		currentMaxBytesPerBatch: maxBytesPerBatch,
	}
}

// BeginBatch sets the opening state before attempting to publish a batch of logs. Specifically, it must
// - note the time (to determine if an error later was timeout related)
// - note whether this batch is full (to determine whether success should increase the limit on success later)
func (lps *logPublicationState) BeginBatch(startTime time.Time, bufferFilled bool) {
	lps.currentBatchStartTime = startTime
	lps.currentBatchBufferFilled = bufferFilled
}

func (lps *logPublicationState) CurrentValues() map[string]int {
	return map[string]int{
		"options_batch_limit_bytes": lps.maxBytesPerBatch,
		"current_batch_limit_bytes": lps.currentMaxBytesPerBatch,
	}
}

func (lps *logPublicationState) EndBatch(logs []string, successful bool) {
	// ensure we reset all batch state at the end
	defer func() {
		lps.currentBatchBufferFilled = false
		lps.currentBatchStartTime = time.Time{}
	}()

	// we can always safely decrease the threshold for a failed batch, but
	// shouldn't increase the threshold for a successful batch unless we've at
	// least filled the buffer
	if successful && !lps.currentBatchBufferFilled {
		return
	}

	// in practice there could be one of a few different transport or timeout errors that bubble up
	// depending on network conditions. instead of trying to keep up with all potential errors,
	// only reduce the threshold if the calls are failing after more than 20 seconds
	if !successful && time.Since(lps.currentBatchStartTime) < maxPublicationDuration {
		return
	}

	if successful {
		lps.increaseBatchThreshold()
		return
	}

	lps.reduceBatchThreshold()
}

func (lps *logPublicationState) ExceedsCurrentBatchThreshold(amountBytes int) bool {
	return amountBytes > lps.currentMaxBytesPerBatch
}

func (lps *logPublicationState) reduceBatchThreshold() {
	if lps.currentMaxBytesPerBatch <= minBytesPerBatch {
		return
	}

	newTargetThreshold := lps.currentMaxBytesPerBatch - batchIncrementAmount
	lps.currentMaxBytesPerBatch = maxInt(newTargetThreshold, minBytesPerBatch)
}

func (lps *logPublicationState) increaseBatchThreshold() {
	if lps.currentMaxBytesPerBatch >= lps.maxBytesPerBatch {
		return
	}

	newTargetThreshold := lps.currentMaxBytesPerBatch + batchIncrementAmount
	lps.currentMaxBytesPerBatch = minInt(newTargetThreshold, lps.maxBytesPerBatch)
}
