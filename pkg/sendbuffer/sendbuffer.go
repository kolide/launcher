package sendbuffer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
)

type sender interface {
	Send(r io.Reader) error
}

var (
	defaultMaxSize     = 512 * 1024
	defaultMaxSendSize = 8 * 1024
)

type SendBuffer struct {
	logs                              [][]byte
	size, maxStorageSize, maxSendSize int
	sendMutex                         sync.Mutex
	writeMutex                        sync.RWMutex
	logger                            log.Logger
	sender                            sender
	sendInterval                      time.Duration
	isSending                         bool
}

type option func(*SendBuffer)

func WithMaxStorageSize(maxSize int) option {
	return func(sb *SendBuffer) {
		sb.maxStorageSize = maxSize
	}
}

func WithMaxSendSize(sendSize int) option {
	return func(sb *SendBuffer) {
		sb.maxSendSize = sendSize
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
		sb.sendInterval = sendInterval
	}
}

func New(sender sender, opts ...option) *SendBuffer {
	sb := &SendBuffer{
		maxStorageSize: defaultMaxSize,
		maxSendSize:    defaultMaxSendSize,
		sender:         sender,
		sendInterval:   1 * time.Minute,
		logger:         log.NewNopLogger(),
		isSending:      false,
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
		return 0, nil
	}

	// if the single data piece is larger than the max send size, drop it and log
	if len(in) > sb.maxSendSize {
		sb.logger.Log(
			"msg", "dropped data because element greater than max send size",
			"size_of_data", len(in),
			"max_send_size", sb.maxSendSize,
			"head", string(in)[0:minInt(len(in), 100)],
		)
		return len(in), nil
	}

	// if we are full, something has backed up
	// purge everything
	if len(in)+sb.size > sb.maxStorageSize {
		sb.deleteLogs(len(sb.logs))

		sb.logger.Log(
			"msg", "reached capacity, dropping all data and starting over",
			"size_of_data", len(in),
			"buffer_size", sb.size,
			"size_plus_data", sb.size+len(in),
			"max_size", sb.maxStorageSize,
		)
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

	ticker := time.NewTicker(sb.sendInterval)
	defer ticker.Stop()

	for {
		if err := sb.sendAndPurge(); err != nil {
			sb.logger.Log("msg", "failed to send and purge", "err", err)
		}

		select {
		case <-ticker.C:
			continue
		case <-ctx.Done():
			return nil
		}
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
	lastKey, err := sb.copyLogs(toSendBuff, sb.maxSendSize)
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
	// TODO: populate logs with device data (id, serial, munemo, orgid) when we
	// get first set of control data with device info before shipping

	// testing on a new enrollment in debug mode, log size hit 130K bytes
	// before enrollment completed and was able to ship logs
	// 2023-11-16
	sb.writeMutex.Lock()
	defer sb.writeMutex.Unlock()
	sb.deleteLogs(lastKey)

	return nil
}

// copyLogs writes to the provided writer until adding another log would exceed
// the provided max size and returns the index of the last log written
// leaving it up to the caller to delete the logs
func (sb *SendBuffer) copyLogs(w io.Writer, maxSize int) (int, error) {
	sb.writeMutex.RLock()
	defer sb.writeMutex.RUnlock()

	size := 0
	lastLogIndex := 0

	for i := 0; i < len(sb.logs); i++ {
		if len(sb.logs[i])+size > maxSize {
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
