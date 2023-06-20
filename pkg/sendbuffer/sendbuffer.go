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
	return func(hb *SendBuffer) {
		hb.maxSize = maxSize
	}
}

func WithMaxSendSize(sendSize int) option {
	return func(hb *SendBuffer) {
		hb.maxSendSize = sendSize
	}
}

func WithLogger(logger log.Logger) option {
	return func(hb *SendBuffer) {
		hb.logger = logger
	}
}

func WithKVStore(kvStore types.KVStore) option {
	return func(hb *SendBuffer) {
		hb.kvStore = kvStore
	}
}

// WithSendInterval sets the interval at which the buffer will send data.
func WithSendInterval(sendInterval time.Duration) option {
	return func(hb *SendBuffer) {
		hb.sendInterval = sendInterval
	}
}

func New(sender sender, opts ...option) *SendBuffer {
	hb := &SendBuffer{
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
		opt(hb)
	}

	hb.logger = log.With(hb.logger, "component", "sendbuffer")

	// get all data keys and calculate starting size
	hb.kvStore.ForEach(func(k, v []byte) error {
		hb.size += len(k) + len(v)
		hb.dataKeys = append(hb.dataKeys, k)
		return nil
	})

	// sort the keys
	sort.Slice(hb.dataKeys, func(i, j int) bool {
		return bytes.Compare(hb.dataKeys[i], hb.dataKeys[j]) < 0
	})

	return hb
}

func (hb *SendBuffer) Write(data []byte) (int, error) {
	hb.writeMutex.Lock()
	defer hb.writeMutex.Unlock()

	if len(data) == 0 {
		return 0, nil
	}

	// if the single data piece is larger than the max send size, drop it and log
	if len(data) > hb.maxSendSize {
		hb.logger.Log(
			"msg", "dropped data because element greater than max send size",
			"size_of_data", len(data),
			"max_send_size", hb.maxSendSize,
			"head", string(data)[0:minInt(len(data), 100)],
		)
		return len(data), nil
	}

	// if we are full, something has backed up
	// purge everything
	if len(data)+hb.size > hb.maxSize {
		hb.logger.Log(
			"msg", "reached capacity, dropping all data and starting over",
			"size_of_data", len(data),
			"buffer_size", hb.size,
			"size_plus_data", hb.size+len(data),
			"max_size", hb.maxSize,
		)

		if err := hb.purge(len(hb.dataKeys)); err != nil {
			// something has gone horribly wrong
			hb.logger.Log("msg", "failed to purge", "err", err)
			return len(data), nil
		}

		hb.size = 0
	}

	dataKey := []byte(ulid.New())
	if err := hb.kvStore.Set(dataKey, data); err != nil {
		// log it and move on
		hb.logger.Log("msg", "failed to store data", "err", err)
		return len(data), nil
	}

	hb.dataKeys = append(hb.dataKeys, dataKey)
	hb.size += len(data) + len(dataKey)
	return len(data), nil
}

func (hb *SendBuffer) StartSending() {
	if hb.isSending {
		return
	}

	hb.isSending = true

	go func() {
		ticker := time.NewTicker(hb.sendInterval)
		defer ticker.Stop()

		for {
			if err := hb.sendAndPurge(); err != nil {
				hb.logger.Log("msg", "failed to send and purge", "err", err)
			}

			select {
			case <-ticker.C:
				continue
			case <-hb.stopSendingChan:
				hb.isSending = false
				return
			}
		}
	}()
}

func (hb *SendBuffer) StopSending() {
	select {
	case hb.stopSendingChan <- struct{}{}:
	default:
	}
}

func (hb *SendBuffer) sendAndPurge() error {
	hb.writeMutex.Lock()
	defer hb.writeMutex.Unlock()

	if len(hb.dataKeys) == 0 {
		return nil
	}

	removeDataKeysToIndex := 0
	sendBuffer := &bytes.Buffer{}
	bytesToDelete := 0

	for _, dataKey := range hb.dataKeys {
		data, err := hb.kvStore.Get(dataKey)
		if err != nil {
			hb.logger.Log("msg", "could not fetch data from kv store", "data_key", dataKey)
			removeDataKeysToIndex++
			continue
		}

		// ready to send, break from loop
		if len(data)+sendBuffer.Len() > hb.maxSendSize {
			break
		}

		// add to buffer to be sent
		if _, err := sendBuffer.Write(data); err != nil {
			return fmt.Errorf("writing data to buffer: %w", err)
		}

		bytesToDelete += len(data) + len(dataKey)
		removeDataKeysToIndex++
	}

	if !hb.sendMutex.TryLock() {
		hb.logger.Log("msg", "could not get lock on send mutex, will retry")
		return nil
	}

	// have lock on write mutex and send mutex at this point
	if err := hb.purge(removeDataKeysToIndex); err != nil {
		hb.sendMutex.Unlock()
		return fmt.Errorf("purging data from kv store: %w", err)
	}

	hb.size -= bytesToDelete

	// send off async
	go func() {
		defer hb.sendMutex.Unlock()
		if err := hb.sender.Send(sendBuffer); err != nil {
			hb.logger.Log("msg", "failed to send, dropping data", "err", err)
		}
	}()

	return nil
}

func (hb *SendBuffer) purge(toIndex int) error {
	keysToRemove := hb.dataKeys[:toIndex]
	if err := hb.kvStore.Delete(keysToRemove...); err != nil {
		return fmt.Errorf("deleting data from kv store: %w", err)
	}

	hb.dataKeys = hb.dataKeys[toIndex:]
	return nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}

	return b
}
