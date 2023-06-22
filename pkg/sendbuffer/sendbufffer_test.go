package sendbuffer

import (
	"bytes"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSendBuffer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                     string
		maxSize, maxSendSize     int
		startingData             []string
		writes, expectedReceives []string
	}{
		{
			name:             "single write, single send",
			maxSize:          1000,
			maxSendSize:      1000,
			writes:           []string{"hello"},
			expectedReceives: []string{"hello"},
		},
		{
			name:             "multiple write, multiple send",
			maxSize:          1000,
			maxSendSize:      1,
			writes:           []string{"0", "1", "2"},
			expectedReceives: []string{"0", "1", "2"},
		},
		{
			name:             "does not exceed max size",
			maxSize:          4,
			maxSendSize:      1000,
			writes:           []string{"hello"},
			expectedReceives: nil,
		},
		{
			name:             "does not exceed max send size",
			maxSize:          1000,
			maxSendSize:      4,
			writes:           []string{"hello"},
			expectedReceives: nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// set up storage, adding starting data
			lastReceivedData := &bytes.Buffer{}

			sb := New(
				&testSender{lastReceived: lastReceivedData, t: t},
				WithMaxSize(tt.maxSize),
				WithMaxSendSize(tt.maxSendSize),
				WithSendInterval(1*time.Hour),
			)

			requireStoreSizeEqualsHttpBufferReportedSize(t, sb)

			for _, write := range tt.writes {
				_, err := sb.Write([]byte(write))
				require.NoError(t, err)
				requireStoreSizeEqualsHttpBufferReportedSize(t, sb)
			}

			for i := 0; i < len(tt.expectedReceives); i++ {
				require.NoError(t, sb.sendAndPurge())

				for {
					// wait for the send to finish
					if sb.sendMutex.TryLock() {
						sb.sendMutex.Unlock()
						break
					}
					time.Sleep(1 * time.Millisecond)
				}

				require.Equal(t, tt.expectedReceives[i], string(lastReceivedData.Bytes()))
				requireStoreSizeEqualsHttpBufferReportedSize(t, sb)
			}
		})
	}
}

func testStringArray(size int) []string {
	arr := make([]string, size)

	for i := 0; i < size; i++ {
		arr[i] = fmt.Sprintf("%d", i)
	}

	return arr
}

func TestSendBufferConcurrent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		maxSize, maxSendSize int
		writes               []string
	}{
		{
			name:        "a little concurrent",
			maxSize:     defaultMaxSize,
			maxSendSize: 1,
			writes:      testStringArray(10),
		},
		{
			name:        "more concurrent",
			maxSize:     defaultMaxSize,
			maxSendSize: 10,
			writes:      testStringArray(100),
		},
		{
			name:        "a lot concurrent",
			maxSize:     defaultMaxSize,
			maxSendSize: 100,
			writes:      testStringArray(1000),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testSender := &testSender{lastReceived: &bytes.Buffer{}, t: t}
			hb := New(
				testSender,
				WithMaxSize(tt.maxSize),
				WithMaxSendSize(tt.maxSendSize),
				// run interval in background quickly
				WithSendInterval(1*time.Millisecond),
			)

			hb.StartSending()

			var wg sync.WaitGroup
			wg.Add(len(tt.writes))
			for _, write := range tt.writes {
				write := write
				go func() {
					_, err := hb.Write([]byte(write))
					require.NoError(t, err)
					wg.Done()
				}()
			}
			// wait for everything to be written
			wg.Wait()

			// check that size reported is correct
			requireStoreSizeEqualsHttpBufferReportedSize(t, hb)

			expectedAggregatedReceives := ""
			for _, write := range tt.writes {
				expectedAggregatedReceives += write
			}

			// make sure were done writing, done sending, and
			// have sent all data
			done := func() bool {
				if !hb.sendMutex.TryLock() {
					return false
				}
				defer hb.sendMutex.Unlock()

				if !hb.writeMutex.TryLock() {
					return false
				}
				defer hb.writeMutex.Unlock()

				return hb.size == 0
			}

			for !done() {
				time.Sleep(10 * time.Millisecond)
			}

			// check that size reported is correct
			requireStoreSizeEqualsHttpBufferReportedSize(t, hb)

			hb.StopSending()

			expected := []rune(expectedAggregatedReceives)
			actual := []rune(string(testSender.aggregateAllReceived()))
			require.ElementsMatch(t, expected, actual)
		})
	}
}

func requireStoreSizeEqualsHttpBufferReportedSize(t *testing.T, hb *SendBuffer) {
	hb.writeMutex.Lock()
	defer hb.writeMutex.Unlock()

	storeSize := 0
	for _, v := range hb.logs {
		storeSize += len(v)
	}

	require.Equal(t, hb.size, storeSize, "actual store size should match buffer size")
}

type testSender struct {
	t            *testing.T
	lastReceived *bytes.Buffer
	allReceived  [][]byte
}

func (s *testSender) Send(r io.Reader) error {
	s.lastReceived.Reset()
	data, err := io.ReadAll(r)
	require.NoError(s.t, err)

	io.Copy(s.lastReceived, bytes.NewReader(data))
	s.allReceived = append(s.allReceived, data)

	return nil
}

func (s *testSender) aggregateAllReceived() []byte {
	var aggregated []byte
	for _, data := range s.allReceived {
		aggregated = append(aggregated, data...)
	}
	return aggregated
}
