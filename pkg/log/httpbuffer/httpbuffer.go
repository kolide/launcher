package httpbuffer

import (
	"bytes"
	"fmt"
	"io"
	"sync"

	"net/http"

	"github.com/go-kit/kit/log"
)

type httpBuffer struct {
	buf                 bytes.Buffer
	bufMutex, sendMutex sync.Mutex
	maxSize, sendSize   int
	endpoint            string
	logger              log.Logger
}

type httpBufferOption func(*httpBuffer)

func WithMaxSize(maxSize int) httpBufferOption {
	return func(hb *httpBuffer) {
		hb.maxSize = maxSize
	}
}

func WithSendSize(sendSize int) httpBufferOption {
	return func(hb *httpBuffer) {
		hb.sendSize = sendSize
	}
}

func WithLogger(logger log.Logger) httpBufferOption {
	return func(hb *httpBuffer) {
		hb.logger = logger
	}
}

func New(endpoint string, opts ...httpBufferOption) *httpBuffer {
	buffer := &httpBuffer{
		maxSize:  128 * 1024,
		sendSize: 8 * 1024,
		endpoint: endpoint,
	}

	for _, opt := range opts {
		opt(buffer)
	}

	return buffer
}

func (hb *httpBuffer) Write(p []byte) (n int, err error) {
	hb.bufMutex.Lock()
	defer hb.bufMutex.Unlock()

	if hb.buf.Len() > hb.maxSize {
		// just drop our logs on the floor and start over
		hb.logger.Log("msg", "buffer is full, dropping logs", "size", hb.buf.Len())
		hb.buf.Reset()
	}

	n, err = hb.buf.Write(p)

	// only try to send if we're at send size and not currently sending
	if hb.buf.Len() >= hb.sendSize && hb.sendMutex.TryLock() {
		sendBuf := &bytes.Buffer{}
		sendBuf.ReadFrom(&hb.buf)

		go func() {
			defer hb.sendMutex.Unlock()
			hb.send(sendBuf)
		}()
	}

	return n, err
}

func (hb *httpBuffer) Flush() error {
	hb.bufMutex.Lock()
	defer hb.bufMutex.Unlock()
	return hb.send(&hb.buf)
}

func (hb *httpBuffer) send(r io.Reader) error {
	resp, err := http.Post(hb.endpoint, "application/octet-stream", r)
	if err != nil {
		return err
	}
	resp.Body.Close()

	// check for status code
	if resp.StatusCode >= 300 {
		return fmt.Errorf("status code: %d", resp.StatusCode)
	}

	return nil
}
