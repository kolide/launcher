package ringlogger

import (
	"bytes"
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

func (rl *ringLogger) GetAll() ([][]byte, error) {
	return rl.ring.GetAll()
}
