package httpbuffer

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHttpBuffer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                                          string
		maxSize, sendSize                             int
		startingData, writeData, expectedDataReceived []byte
	}{
		{
			name:      "data below send size, gets flushed",
			sendSize:  6,
			writeData: []byte("hello"),
		},
		{
			name:                 "data above send size, below max size",
			sendSize:             3,
			writeData:            []byte("hello"),
			expectedDataReceived: []byte("hello"),
		},
		{
			name:                 "starting data is above max size, gets dropped",
			maxSize:              4,
			startingData:         []byte("this is way above the max size"),
			writeData:            []byte("hello"),
			expectedDataReceived: []byte("hello"),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var receivedData []byte

			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				var err error
				receivedData, err = ioutil.ReadAll(req.Body)
				assert.NoError(t, err)
				rw.Write([]byte("OK"))
			}))

			defer server.Close()

			hb := New(server.URL, WithMaxSize(tt.maxSize), WithSendSize(tt.sendSize))
			hb.buf = *bytes.NewBuffer(tt.startingData)

			_, err := hb.Write(tt.writeData)
			assert.NoError(t, err)

			// wait until it's done sending
			for {
				if hb.sendMutex.TryLock() {
					hb.sendMutex.Unlock()
					break
				}
				time.Sleep(100 * time.Millisecond)
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedDataReceived, receivedData, "received data from first send does not match expected data")

			// calculate expected flush data as the remainder of the data after the expected output
			expectedFlush := tt.writeData[len(tt.expectedDataReceived):]

			// reset receivedData for the next send
			receivedData = []byte{}

			// call Flush to send any remaining data
			err = hb.Flush()
			assert.NoError(t, err)
			assert.Equal(t, expectedFlush, receivedData, "received data from flush does not match expected")

			// make sure buffers is empty
			assert.Equal(t, 0, hb.buf.Len())
		})
	}
}
