package sendbuffer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

type sender interface {
	Send(r io.Reader) error
}

var (
	defaultMaxSizeBytes  = 512 * 1024
	defaultSendSizeBytes = 8 * 1024
)

type SendBuffer struct {
	logs                                        [][]byte
	size, maxStorageSizeBytes, maxSendSizeBytes int
	sendMutex                                   sync.Mutex
	writeMutex                                  sync.RWMutex
	logger                                      log.Logger
	sender                                      sender
	sendTicker                                  *time.Ticker
	isSending                                   bool

	// logsJustPurged is used to prevent attempting to delete logs that were just purged
	logsJustPurged bool
}

type option func(*SendBuffer)

func WithMaxStorageSizeBytes(maxSize int) option {
	return func(sb *SendBuffer) {
		sb.maxStorageSizeBytes = maxSize
	}
}

func WithMaxSendSizeBytes(sendSize int) option {
	return func(sb *SendBuffer) {
		sb.maxSendSizeBytes = sendSize
	}
}

func WithLogger(logger log.Logger) option {
	return func(sb *SendBuffer) {
		sb.logger = logger
	}
}

// WithSendInterval sets the interval at which the buffer will send data.
func WithSendInterval(sendInterval time.Duration) option {
	return func(sb *SendBuffer) {
		sb.sendTicker.Reset(sendInterval)
	}
}

func New(sender sender, opts ...option) *SendBuffer {
	sb := &SendBuffer{
		maxStorageSizeBytes: defaultMaxSizeBytes,
		maxSendSizeBytes:    defaultSendSizeBytes,
		sender:              sender,
		sendTicker:          time.NewTicker(1 * time.Minute),
		logger:              log.NewNopLogger(),
		isSending:           false,
	}

	for _, opt := range opts {
		opt(sb)
	}

	sb.logger = log.With(sb.logger, "component", "sendbuffer")

	return sb
}

func (sb *SendBuffer) Write(in []byte) (int, error) {
	sb.writeMutex.Lock()
	defer sb.writeMutex.Unlock()

	if len(in) == 0 {
		sb.logger.Log(
			"msg", "dropped data because element was empty",
			"method", "UpdateData",
		)

		return len(in), nil
	}

	// if the single data piece is larger than the max send size, drop it and log
	if len(in) > sb.maxSendSizeBytes {
		sb.logger.Log(
			"msg", "dropped data because element greater than max send size",
			"method", "Write",
			"size_of_data_bytes", len(in),
			"max_send_size_bytes", sb.maxSendSizeBytes,
			"head", string(in)[0:minInt(len(in), 100)],
		)
		return len(in), nil
	}

	// if we are full, something has backed up
	// purge everything
	if len(in)+sb.size > sb.maxStorageSizeBytes {
		sb.deleteLogs(len(sb.logs))

		// mark that we have just purged the logs so that any waiting deletes
		// will not try to delete what was purged
		sb.logsJustPurged = true

		sb.logger.Log(
			"msg", "reached capacity, dropping all data and starting over",
			"method", "Write",
			"size_of_data_bytes", len(in),
			"buffer_size_bytes", sb.size,
			"size_plus_data_bytes", sb.size+len(in),
			"max_size", sb.maxStorageSizeBytes,
		)

		return len(in), nil
	}

	// If we don't make a copy of the data, we get data loss in the logs array.
	// It seems the input gets concurrenlty overridden somewhere under the hood.
	data := make([]byte, len(in))
	copy(data, in)

	sb.logs = append(sb.logs, data)
	sb.size += len(data)
	return len(in), nil
}

func (sb *SendBuffer) Run(ctx context.Context) error {
	if sb.isSending {
		return errors.New("already running")
	}

	sb.isSending = true
	defer func() {
		sb.isSending = false
	}()

	for {
		if err := sb.sendAndPurge(); err != nil {
			sb.logger.Log("msg", "failed to send and purge", "err", err)
		}

		select {
		case <-sb.sendTicker.C:
			continue
		case <-ctx.Done():
			// Send one final batch, if possible, so that we can get logs related to shutdowns.
			// Sleep for one second first to allow any shutdown-related logs to be written.
			time.Sleep(1 * time.Second)
			if err := sb.sendAndPurge(); err != nil {
				sb.logger.Log("msg", "failed to send final batch of logs on shutdown", "err", err)
			}
			return nil
		}
	}
}

func (sb *SendBuffer) SetSendInterval(sendInterval time.Duration) {
	sb.sendTicker.Reset(sendInterval)
}

func (sb *SendBuffer) UpdateData(f func(in io.Reader, out io.Writer) error) {
	sb.writeMutex.Lock()
	defer sb.writeMutex.Unlock()

	var indexesToDelete []int

	for i := 0; i < len(sb.logs); i++ {
		in := bytes.NewReader(sb.logs[i])
		out := &bytes.Buffer{}

		inSize := in.Len()

		// do the update, if it fails, preserve data
		if err := f(in, out); err != nil {
			level.Debug(sb.logger).Log(
				"msg", "update function failed, preserving original data",
				"method", "UpdateData",
				"err", err,
			)

			continue
		}

		// subtract original size, wait until after update func is called
		// incase it fails, we don't want to modify size
		sb.size -= inSize
		sb.logs[i] = nil

		outLen := out.Len()

		// if the new length is 0, mark for deletion
		if outLen == 0 {
			indexesToDelete = append(indexesToDelete, i)

			level.Debug(sb.logger).Log(
				"msg", "dropped data because element was empty",
				"method", "UpdateData",
			)

			continue
		}

		// if new size excceds max send size, mark for deletion
		if outLen > sb.maxSendSizeBytes {
			indexesToDelete = append(indexesToDelete, i)

			level.Debug(sb.logger).Log(
				"msg", "dropped data because element greater than max send size",
				"method", "UpdateData",
				"size_of_data_bytes", out.Len(),
				"max_send_size_bytes", sb.maxSendSizeBytes,
				"head", out.String()[0:minInt(outLen, 100)],
			)

			continue
		}

		// if new size exceeds max storage size, mark for deletion
		if outLen+sb.size > sb.maxStorageSizeBytes {
			indexesToDelete = append(indexesToDelete, i)

			// log it
			sb.logger.Log(
				"msg", "dropped data because buffer full",
				"method", "UpdateData",
				"size_of_data_bytes", outLen,
				"buffer_size_bytes", sb.size,
				"size_plus_data_bytes", sb.size+outLen,
				"max_size", sb.maxStorageSizeBytes,
				"head", out.String()[0:minInt(outLen, 100)],
			)

			continue
		}

		// update log and size
		sb.logs[i] = out.Bytes()
		sb.size += outLen
	}

	// remove indexes marked for deletion
	for i := 0; i < len(indexesToDelete); i++ {
		// shift left by i each time we delete an element to accout for decreased length
		indexToDelete := indexesToDelete[i] - i
		sb.logs = append(sb.logs[:indexToDelete], sb.logs[indexToDelete+1:]...)
	}
}

func (sb *SendBuffer) DeleteAllData() {
	sb.writeMutex.Lock()
	defer sb.writeMutex.Unlock()
	sb.logs = nil
	sb.size = 0
}

func (sb *SendBuffer) sendAndPurge() error {
	if !sb.sendMutex.TryLock() {
		sb.logger.Log("msg", "could not get lock on send mutex, will retry")
		return nil
	}
	defer sb.sendMutex.Unlock()

	toSendBuff := &bytes.Buffer{}
	lastKey, err := sb.copyLogs(toSendBuff, sb.maxSendSizeBytes)
	if err != nil {
		return err
	}

	if toSendBuff.Len() == 0 {
		return nil
	}

	if err := sb.sender.Send(toSendBuff); err != nil {
		sb.logger.Log("msg", "failed to send, will retry", "err", err)
		return nil
	}

	// testing on a new enrollment in debug mode, log size hit 130K bytes
	// before enrollment completed and was able to ship logs
	// 2023-11-16
	sb.writeMutex.Lock()
	defer sb.writeMutex.Unlock()

	// There is a possibility that the log buffer gets full while were in the middle of sending
	// and gets deleted. However, we don't want to block writes while were waiting on a network call
	// to send the logs. To live with this, we just verify that the logs didn't just get purged.
	if sb.logsJustPurged {
		sb.logsJustPurged = false
		return nil
	}

	sb.deleteLogs(lastKey)

	return nil
}

// copyLogs writes to the provided writer, peeking at the size of each log
// before for copying and returning when the next log would exceed the maxSize,
// it's up to the caller to delete any copied logs
func (sb *SendBuffer) copyLogs(w io.Writer, maxSizeBytes int) (int, error) {
	sb.writeMutex.RLock()
	defer sb.writeMutex.RUnlock()

	size := 0
	lastLogIndex := 0

	for i := 0; i < len(sb.logs); i++ {
		if len(sb.logs[i])+size > maxSizeBytes {
			break
		}

		if _, err := w.Write(sb.logs[i]); err != nil {
			return 0, err
		}

		size += len(sb.logs[i])
		lastLogIndex++
	}

	return lastLogIndex, nil
}

// deleteLogs deletes the logs up to the provided index
// it's up to the caller to lock the write mutex
func (sb *SendBuffer) deleteLogs(toIndex int) {
	sizeDeleted := 0
	for i := 0; i < toIndex; i++ {
		sizeDeleted += len(sb.logs[i])
	}

	sb.logs = sb.logs[toIndex:]
	sb.size -= sizeDeleted
}

func minInt(a, b int) int {
	if a < b {
		return a
	}

	return b
}
