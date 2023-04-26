package ringlogger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/go-kit/kit/log"
)

type ringer interface {
	Add(val []byte) (err error)
	GetAll() ([][]byte, error)
}

type ringLogger struct {
	ring   ringer
	logger log.Logger
	buf    *bytes.Buffer
	lock   sync.Mutex
}

func New(ring ringer) (*ringLogger, error) {
	buf := &bytes.Buffer{}
	rl := &ringLogger{
		ring:   ring,
		buf:    buf,
		logger: log.NewJSONLogger(buf),
	}

	return rl, nil
}

func (rl *ringLogger) Log(keyvals ...interface{}) error {
	rl.lock.Lock()
	defer rl.lock.Unlock()

	rl.buf.Reset()
	if err := rl.logger.Log(keyvals...); err != nil {
		return fmt.Errorf("writing logs to ringlogger buffer: %w", err)
	}

	if err := rl.ring.Add(rl.buf.Bytes()); err != nil {
		return fmt.Errorf("writing logs to ring: %w", err)
	}

	return nil
}

func (rl *ringLogger) GetAll() ([]map[string]any, error) {

	all, err := rl.ring.GetAll()
	if err != nil {
		return nil, fmt.Errorf("getting all logs from ring: %w", err)
	}

	logs := make([]map[string]any, len(all))

	reader := &bytes.Reader{}
	dec := json.NewDecoder(reader)

	for i, logLineRaw := range all {
		var logLine map[string]any
		reader.Reset(logLineRaw)
		if err := dec.Decode(&logLine); err != nil {
			return nil, fmt.Errorf("decoding stored log: %w", err)
		}

		logs[i] = logLine
	}

	return logs, nil
}
