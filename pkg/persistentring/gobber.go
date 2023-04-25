package persistentring

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"sync"

	"github.com/davecgh/go-spew/spew"
)

type gobber struct {
	encBuf  bytes.Buffer // pointer or real?
	encLock sync.Mutex
	decBuf  bytes.Buffer // pointer or real?
	decLock sync.Mutex

	enc *gob.Encoder
	dec *gob.Decoder
}

func NewGobber() *gobber {
	e := gobber{}
	e.enc = gob.NewEncoder(&e.encBuf)
	e.dec = gob.NewDecoder(&e.decBuf)

	return &e
}

func (e *gobber) Encode(v interface{}) ([]byte, error) {
	e.encLock.Lock()
	defer e.encLock.Unlock()

	e.encBuf.Reset()
	if err := e.enc.Encode(v); err != nil {
		return nil, fmt.Errorf("encoding: %w", err)
	}

	return e.encBuf.Bytes(), nil
}

func (e *gobber) Decode(b []byte, v interface{}) error {
	e.decLock.Lock()
	defer e.decLock.Unlock()

	e.decBuf.Reset()
	if _, err := e.decBuf.Write(b); err != nil {
		return fmt.Errorf("writing bytes to buffer: %w", err)
	}

	spew.Dump(e.decBuf.Bytes())
	rc := e.dec.Decode(v)
	spew.Dump(v)
	return rc

}
