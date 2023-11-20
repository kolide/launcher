package sendbuffer

import (
	"bytes"
	"context"
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
		name                        string
		maxStorageSize, maxSendSize int
		writes, expectedReceives    []string
	}{
		{
			name:             "single write, single send",
			maxStorageSize:   1000,
			maxSendSize:      1000,
			writes:           []string{"hello"},
			expectedReceives: []string{"hello"},
		},
		{
			name:             "multiple write, multiple send",
			maxStorageSize:   1000,
			maxSendSize:      2,
			writes:           []string{"01", "2", "3", "4"},
			expectedReceives: []string{"01", "23", "4"},
		},
		{
			name:             "multiple write, single send",
			maxStorageSize:   1000,
			maxSendSize:      3,
			writes:           []string{"0", "1", "2"},
			expectedReceives: []string{"012"},
		},
		{
			name:             "does not exceed max size",
			maxStorageSize:   4,
			maxSendSize:      1000,
			writes:           []string{"hello"},
			expectedReceives: nil,
		},
		{
			name:             "does not exceed max send size",
			maxStorageSize:   1000,
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
				WithMaxStorageSizeBytes(tt.maxStorageSize),
				WithMaxSendSizeBytes(tt.maxSendSize),
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
		name        string
		maxSendSize int
		writes      []string
	}{
		{
			name:        "a little concurrent",
			maxSendSize: 1,
			writes:      testStringArray(10),
		},
		{
			name:        "more concurrent",
			maxSendSize: 10,
			writes:      testStringArray(100),
		},
		{
			name:        "a lot concurrent",
			maxSendSize: 100,
			writes:      testStringArray(1000),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testSender := &testSender{lastReceived: &bytes.Buffer{}, t: t}
			sb := New(
				testSender,
				WithMaxSendSizeBytes(tt.maxSendSize),
				// run interval in background quickly
				WithSendInterval(1*time.Millisecond),
			)

			go func() {
				sb.Run(context.Background())
			}()

			var wg sync.WaitGroup
			wg.Add(len(tt.writes))
			for _, write := range tt.writes {
				write := write
				go func() {
					_, err := sb.Write([]byte(write))
					require.NoError(t, err)
					wg.Done()
				}()
			}
			// wait for everything to be written
			wg.Wait()

			// check that size reported is correct
			requireStoreSizeEqualsHttpBufferReportedSize(t, sb)

			expectedAggregatedReceives := ""
			for _, write := range tt.writes {
				expectedAggregatedReceives += write
			}

			// make sure were done writing, done sending, and
			// have sent all data
			done := func() bool {
				if !sb.sendMutex.TryLock() {
					return false
				}
				defer sb.sendMutex.Unlock()

				if !sb.writeMutex.TryLock() {
					return false
				}
				defer sb.writeMutex.Unlock()

				return sb.size == 0
			}

			for !done() {
				time.Sleep(10 * time.Millisecond)
			}

			// check that size reported is correct
			requireStoreSizeEqualsHttpBufferReportedSize(t, sb)

			expected := []rune(expectedAggregatedReceives)
			actual := []rune(string(testSender.aggregateAllReceived()))
			require.ElementsMatch(t, expected, actual)
		})
	}
}

func TestSendBuffer_DeleteAllData(t *testing.T) {
	t.Parallel()

	testSender := &testSender{lastReceived: &bytes.Buffer{}, t: t}
	sb := New(
		testSender,
	)

	sb.Write([]byte("here is some data"))

	require.NotEmpty(t, sb.logs)
	require.NotZero(t, sb.size)

	sb.DeleteAllData()

	require.Empty(t, sb.logs)
	require.Zero(t, sb.size)
}

func requireStoreSizeEqualsHttpBufferReportedSize(t *testing.T, sb *SendBuffer) {
	sb.writeMutex.Lock()
	defer sb.writeMutex.Unlock()

	storeSize := 0
	for _, v := range sb.logs {
		storeSize += len(v)
	}

	require.Equal(t, sb.size, storeSize, "actual store size should match buffer size")
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
