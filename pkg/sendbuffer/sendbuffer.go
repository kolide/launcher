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
	defaultMaxSize     = 128 * 1024
	defaultMaxSendSize = 8 * 1024
)

type SendBuffer struct {
	logs                              [][]byte
	size, maxStorageSize, maxSendSize int
	writeMutex, sendMutex             sync.Mutex
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

func (sb *SendBuffer) Write(data []byte) (int, error) {
	sb.writeMutex.Lock()
	defer sb.writeMutex.Unlock()

	if len(data) == 0 {
		return 0, nil
	}

	// if the single data piece is larger than the max send size, drop it and log
	if len(data) > sb.maxSendSize {
		sb.logger.Log(
			"msg", "dropped data because element greater than max send size",
			"size_of_data", len(data),
			"max_send_size", sb.maxSendSize,
			"head", string(data)[0:minInt(len(data), 100)],
		)
		return len(data), nil
	}

	// if we are full, something has backed up
	// purge everything
	if len(data)+sb.size > sb.maxStorageSize {
		sb.deleteLogs(len(sb.logs))

		sb.logger.Log(
			"msg", "reached capacity, dropping all data and starting over",
			"size_of_data", len(data),
			"buffer_size", sb.size,
			"size_plus_data", sb.size+len(data),
			"max_size", sb.maxStorageSize,
		)
	}

	sb.addLog(data)
	return len(data), nil
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
			break
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
	if err := sb.flushToWriter(toSendBuff); err != nil {
		return err
	}

	if toSendBuff.Len() == 0 {
		return nil
	}

	if err := sb.sender.Send(toSendBuff); err != nil {
		sb.logger.Log("msg", "failed to send, dropping data", "err", err)
	}

	return nil
}

func (sb *SendBuffer) flushToWriter(w io.Writer) error {
	sb.writeMutex.Lock()
	defer sb.writeMutex.Unlock()

	size := 0
	removeDataKeysToIndex := 0

	for i := 0; i < len(sb.logs); i++ {
		if len(sb.logs[i])+size > sb.maxSendSize {
			break
		}

		if _, err := w.Write(sb.logs[i]); err != nil {
			return err
		}

		size += len(sb.logs[i])
		removeDataKeysToIndex++
	}

	sb.deleteLogs(removeDataKeysToIndex)
	return nil
}

func (sb *SendBuffer) deleteLogs(toIndex int) {
	sizeDeleted := 0
	for i := 0; i < toIndex; i++ {
		sizeDeleted += len(sb.logs[i])
	}

	sb.logs = sb.logs[toIndex:]
	sb.size -= sizeDeleted
}

func (sb *SendBuffer) addLog(data []byte) {
	sb.logs = append(sb.logs, data)
	sb.size += len(data)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}

	return b
}
