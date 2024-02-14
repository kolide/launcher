package osquery

const (
	// set minimum batch size to 0.5 mb as lower bound for correction
	minBytesPerBatch = 524288
)

// logPublicationState holds stateful logic to influence the log batch publication size
// depending on prior successes or failures. The primary intent here is to prevent repeatedly
// consuming the entire network bandwidth available to devices that are unable to ship
// the standard maxBytesPerBatch inside of the connection timeout (currently 30 seconds, enforced cloud side)
type (
	logPublicationState struct {
		// maxBytesPerBatch is passed in from the extension opts and respected
		// as a fixed upper limit for batch size, regardless of publication success/failure rates
		maxBytesPerBatch            int
		currentMaxBytesPerBatch     int
		currentPendingBatch         *logPublicationBatch
		publishedBatches            []*logPublicationBatch
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
		publishedBatches:        make([]*logPublicationBatch, 0),
	}
}

func (lps *logPublicationState) exceedsCurrentBatchThreshold(amountBytes int) bool {
	return amountBytes > lps.currentMaxBytesPerBatch
}

func (lps *logPublicationState) addPendingBatch(amountBytes int) {
	lps.currentPendingBatch = &logPublicationBatch{
		batchSizeBytes: amountBytes,
	}
}

func (lps *logPublicationState) noteBatchComplete(timedOut bool) {
	lps.currentPendingBatch.timedOut = timedOut
	lps.publishedBatches = append(lps.publishedBatches, lps.currentPendingBatch)
	if len(lps.publishedBatches) > 10 {
		lps.publishedBatches = lps.publishedBatches[1:]
	}

	if timedOut {
		lps.reduceBatchThreshold()
	} else {
		lps.increaseBatchThreshold()
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
