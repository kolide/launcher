package sendbuffer

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/pkg/agent/storage/inmemory"
	"github.com/kolide/launcher/pkg/agent/types"
)

type sender interface {
	Send(r io.Reader) error
}

var (
	defaultMaxSize     = 128 * 1024
	defaultMaxSendSize = 8 * 1024
)

type SendBuffer struct {
	kvStore                    types.KVStore
	dataKeys                   [][]byte
	size, maxSize, maxSendSize int
	writeMutex, sendMutex      sync.Mutex
	logger                     log.Logger
	sender                     sender
	sendInterval               time.Duration
	stopSendingChan            chan struct{}
	isSending                  bool
}

type option func(*SendBuffer)

func WithMaxSize(maxSize int) option {
	return func(sb *SendBuffer) {
		sb.maxSize = maxSize
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

func WithKVStore(kvStore types.KVStore) option {
	return func(sb *SendBuffer) {
		sb.kvStore = kvStore
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
		maxSize:         defaultMaxSize,
		maxSendSize:     defaultMaxSendSize,
		dataKeys:        make([][]byte, 0),
		sender:          sender,
		sendInterval:    5 * time.Second,
		logger:          log.NewNopLogger(),
		kvStore:         inmemory.NewStore(log.NewNopLogger()),
		stopSendingChan: make(chan struct{}),
		isSending:       false,
	}

	for _, opt := range opts {
		opt(sb)
	}

	sb.logger = log.With(sb.logger, "component", "sendbuffer")

	// get all data keys and calculate starting size
	sb.kvStore.ForEach(func(k, v []byte) error {
		sb.size += len(k) + len(v)
		sb.dataKeys = append(sb.dataKeys, k)
		return nil
	})

	// sort the keys
	sort.Slice(sb.dataKeys, func(i, j int) bool {
		return bytes.Compare(sb.dataKeys[i], sb.dataKeys[j]) < 0
	})

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
	if len(data)+sb.size > sb.maxSize {
		sb.logger.Log(
			"msg", "reached capacity, dropping all data and starting over",
			"size_of_data", len(data),
			"buffer_size", sb.size,
			"size_plus_data", sb.size+len(data),
			"max_size", sb.maxSize,
		)

		if err := sb.purge(len(sb.dataKeys)); err != nil {
			// something has gone horribly wrong
			sb.logger.Log("msg", "failed to purge", "err", err)
			return len(data), nil
		}

		sb.size = 0
	}

	dataKey := []byte(ulid.New())
	if err := sb.kvStore.Set(dataKey, data); err != nil {
		// log it and move on
		sb.logger.Log("msg", "failed to store data", "err", err)
		return len(data), nil
	}

	sb.dataKeys = append(sb.dataKeys, dataKey)
	sb.size += len(data) + len(dataKey)
	return len(data), nil
}

func (sb *SendBuffer) StartSending() {
	if sb.isSending {
		return
	}

	sb.isSending = true

	go func() {
		ticker := time.NewTicker(sb.sendInterval)
		defer ticker.Stop()

		for {
			if err := sb.sendAndPurge(); err != nil {
				sb.logger.Log("msg", "failed to send and purge", "err", err)
			}

			select {
			case <-ticker.C:
				continue
			case <-sb.stopSendingChan:
				sb.isSending = false
				return
			}
		}
	}()
}

func (sb *SendBuffer) StopSending() {
	select {
	case sb.stopSendingChan <- struct{}{}:
	default:
	}
}

func (sb *SendBuffer) sendAndPurge() error {
	sb.writeMutex.Lock()
	defer sb.writeMutex.Unlock()

	if len(sb.dataKeys) == 0 {
		return nil
	}

	removeDataKeysToIndex := 0
	sendBuffer := &bytes.Buffer{}
	bytesToDelete := 0

	for _, dataKey := range sb.dataKeys {
		data, err := sb.kvStore.Get(dataKey)
		if err != nil {
			sb.logger.Log("msg", "could not fetch data from kv store", "data_key", dataKey)
			removeDataKeysToIndex++
			continue
		}

		// ready to send, break from loop
		if len(data)+sendBuffer.Len() > sb.maxSendSize {
			break
		}

		// add to buffer to be sent
		if _, err := sendBuffer.Write(data); err != nil {
			return fmt.Errorf("writing data to buffer: %w", err)
		}

		bytesToDelete += len(data) + len(dataKey)
		removeDataKeysToIndex++
	}

	if !sb.sendMutex.TryLock() {
		sb.logger.Log("msg", "could not get lock on send mutex, will retry")
		return nil
	}

	// have lock on write mutex and send mutex at this point
	if err := sb.purge(removeDataKeysToIndex); err != nil {
		sb.sendMutex.Unlock()
		return fmt.Errorf("purging data from kv store: %w", err)
	}

	sb.size -= bytesToDelete

	// send off async
	go func() {
		defer sb.sendMutex.Unlock()
		if err := sb.sender.Send(sendBuffer); err != nil {
			sb.logger.Log("msg", "failed to send, dropping data", "err", err)
		}
	}()

	return nil
}

func (sb *SendBuffer) purge(toIndex int) error {
	keysToRemove := sb.dataKeys[:toIndex]
	if err := sb.kvStore.Delete(keysToRemove...); err != nil {
		return fmt.Errorf("deleting data from kv store: %w", err)
	}

	sb.dataKeys = sb.dataKeys[toIndex:]
	return nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}

	return b
}
