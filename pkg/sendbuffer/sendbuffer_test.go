package sendbuffer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

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
			name:             "does not exceed max write size",
			maxStorageSize:   2 * maxWriteBytes,
			maxSendSize:      2 * maxWriteBytes,
			writes:           []string{strings.Repeat("a", maxWriteBytes+1)},
			expectedReceives: nil,
		},
		{
			name:             "drops empty",
			maxStorageSize:   1000,
			maxSendSize:      4,
			writes:           []string{""},
			expectedReceives: nil,
		},
	}

	for _, tt := range tests {
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
				_, err := sb.sendAndPurge(context.Background())
				require.NoError(t, err)

				for {
					// wait for the send to finish
					if sb.sendMutex.TryLock() {
						sb.sendMutex.Unlock()
						break
					}
					time.Sleep(1 * time.Millisecond)
				}

				require.Equal(t, tt.expectedReceives[i], lastReceivedData.String())
				requireStoreSizeEqualsHttpBufferReportedSize(t, sb)
			}
		})
	}
}

// TestSendAndPurgeHandlesLogBufferFullPurge resulted from a panic found in production where
// if the buffer was full while sendAndPurge was running, the buffer get wiped and then
// sendAndPurge would try to delete the portion of the buffer it just sent, causing a panic
func TestSendAndPurgeHandlesLogBufferFullPurge(t *testing.T) {
	t.Parallel()

	sb := New(
		&testSender{lastReceived: &bytes.Buffer{}, t: t},
		WithMaxStorageSizeBytes(11),
		WithMaxSendSizeBytes(5),
		WithSendInterval(100*time.Millisecond),
	)

	// kind of an ugly test, but it was the simplest way to reproduce the issue
	// if the issue is present, we'll get a panic: runtime error: index out of range [x] with length x

	testCompleted := &atomic.Bool{}
	go func() {
		for !testCompleted.Load() {
			sb.Write([]byte("1"))
		}
	}()

	go func() {
		for !testCompleted.Load() {
			time.Sleep(50 * time.Millisecond)
			sb.sendAndPurge(context.Background())
		}
	}()

	time.Sleep(1 * time.Second)

	testCompleted.Store(true)
}

func testStringArray(size int) []string {
	arr := make([]string, size)

	for i := range size {
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
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testSender := &testSender{lastReceived: &bytes.Buffer{}, t: t}
			sb := New(
				testSender,
				WithMaxSendSizeBytes(tt.maxSendSize),
				// run interval in background quickly
				WithSendInterval(1*time.Millisecond),
			)

			runDone := make(chan struct{})
			go func() {
				defer close(runDone)
				sb.Run(t.Context())
			}()

			var wg sync.WaitGroup
			wg.Add(len(tt.writes))
			for _, write := range tt.writes {
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

			var receives strings.Builder
			for _, write := range tt.writes {
				receives.WriteString(write) // Efficient
			}
			expectedAggregatedReceives := receives.String()

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

			// Wait for Run to finish its cleanup after t.Context() is cancelled
			t.Cleanup(func() {
				select {
				case <-runDone:
				case <-time.After(5 * time.Second):
					t.Error("Run did not return after context cancellation")
				}
			})
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

func TestUpdateData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		maxStorageSize   int
		maxSendSize      int
		initialLogs      [][]byte
		updateFunction   func(in io.Reader, out io.Writer) error
		expectedLogs     [][]byte
		expectedSize     int
		expectedLogCount int
	}{
		{
			name:           "happy path",
			maxSendSize:    5,
			maxStorageSize: 10,
			initialLogs: [][]byte{
				[]byte("abcde"),
				[]byte("fghij"),
			},
			updateFunction: func(in io.Reader, out io.Writer) error {
				data, err := io.ReadAll(in)
				require.NoError(t, err)
				_, err = out.Write(bytes.ToUpper(data))
				require.NoError(t, err)
				return err
			},
			expectedLogs: [][]byte{
				[]byte("ABCDE"),
				[]byte("FGHIJ"),
			},
			expectedSize:     10,
			expectedLogCount: 2,
		},
		{
			name:           "exceeds max send size",
			maxSendSize:    1,
			maxStorageSize: 10,
			initialLogs: [][]byte{
				[]byte("0"),
				[]byte("1"),
				[]byte("2"),
				[]byte("3"),
				[]byte("4"),
				[]byte("5"),
			},
			updateFunction: func(in io.Reader, out io.Writer) error {
				data, err := io.ReadAll(in)
				require.NoError(t, err)

				// convert data to int
				numStr := string(data)

				// convert to int
				num, err := strconv.Atoi(numStr)
				require.NoError(t, err)

				// if even, make it exceed max send size
				if num%2 == 0 {
					_, err := out.Write([]byte("TOO_BIG"))
					require.NoError(t, err)
					return err
				}

				_, err = fmt.Fprint(out, num)
				require.NoError(t, err)
				return err
			},
			expectedLogs: [][]byte{
				[]byte("1"),
				[]byte("3"),
				[]byte("5"),
			},
			expectedSize:     3,
			expectedLogCount: 3,
		},
		{
			name:           "exceeds max storage size",
			maxSendSize:    10,
			maxStorageSize: 10,
			initialLogs: [][]byte{
				[]byte("01234"),
				[]byte("01234"),
			},
			updateFunction: func(in io.Reader, out io.Writer) error {
				// this should cause second log to be deleted since the first update
				// will put it over the threshold
				_, err := out.Write([]byte("0123456789"))
				require.NoError(t, err)
				return err
			},
			expectedLogs: [][]byte{
				[]byte("0123456789"),
			},
			expectedSize:     10,
			expectedLogCount: 1,
		},
		{
			name:           "zero lengths",
			maxSendSize:    1,
			maxStorageSize: 10,
			initialLogs: [][]byte{
				[]byte("0"),
				[]byte("1"),
				[]byte("2"),
				[]byte("3"),
				[]byte("4"),
				[]byte("5"),
			},
			updateFunction: func(in io.Reader, out io.Writer) error {
				data, err := io.ReadAll(in)
				require.NoError(t, err)

				numStr := string(data)
				num, err := strconv.Atoi(numStr)
				require.NoError(t, err)

				// if odd, set it to zero
				if num%2 != 0 {
					_, err := out.Write(make([]byte, 0))
					require.NoError(t, err)
					return err
				}

				_, err = fmt.Fprint(out, num)
				require.NoError(t, err)
				return err
			},
			expectedLogs: [][]byte{
				[]byte("0"),
				[]byte("2"),
				[]byte("4"),
			},
			expectedSize:     3,
			expectedLogCount: 3,
		},
		{
			name:           "handles update errors",
			maxSendSize:    1,
			maxStorageSize: 10,
			initialLogs: [][]byte{
				[]byte("0"),
				[]byte("1"),
				[]byte("2"),
				[]byte("3"),
				[]byte("4"),
				[]byte("5"),
			},
			updateFunction: func(in io.Reader, out io.Writer) error {
				data, err := io.ReadAll(in)
				require.NoError(t, err)

				numStr := string(data)
				num, err := strconv.Atoi(numStr)
				require.NoError(t, err)

				// if odd, return error
				if num%2 != 0 {
					return errors.New("some error")
				}

				_, err = fmt.Fprint(out, num)
				require.NoError(t, err)
				return err
			},
			expectedLogs: [][]byte{
				[]byte("0"),
				[]byte("1"),
				[]byte("2"),
				[]byte("3"),
				[]byte("4"),
				[]byte("5"),
			},
			expectedSize:     6,
			expectedLogCount: 6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require := require.New(t)

			testSender := &testSender{lastReceived: &bytes.Buffer{}, t: t}
			sb := New(
				testSender,
				WithMaxSendSizeBytes(tt.maxSendSize),
				WithMaxStorageSizeBytes(tt.maxStorageSize),
			)

			for _, log := range tt.initialLogs {
				_, err := sb.Write(log)
				require.NoError(err)
			}

			sb.UpdateData(tt.updateFunction)

			require.Equal(tt.expectedLogs, sb.logs, "logs not as expected")
			require.Equal(tt.expectedSize, sb.size, "size not as expected")
			require.Equal(tt.expectedLogCount, len(sb.logs), "log count not as expected")
			requireStoreSizeEqualsHttpBufferReportedSize(t, sb)
		})
	}
}

type testSender struct {
	t            *testing.T
	lastReceived *bytes.Buffer
	allReceived  [][]byte
}

func (s *testSender) Send(_ context.Context, r io.Reader) error {
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

// thread-safe sender for tests that tracks stats/sent bytes
type drainTestSender struct {
	mu       sync.Mutex
	sends    int
	received []byte
	err      error
}

func (s *drainTestSender) Send(_ context.Context, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.sends++
	if s.err != nil {
		return s.err
	}

	s.received = append(s.received, data...)
	return nil
}

func (s *drainTestSender) sendCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sends
}

func (s *drainTestSender) receivedData() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return string(s.received)
}

func TestDrain(t *testing.T) {
	t.Parallel()

	writeLines := func(t *testing.T, sb *SendBuffer, lines ...string) {
		t.Helper()
		for _, line := range lines {
			_, err := sb.Write([]byte(line))
			require.NoError(t, err)
		}
	}

	t.Run("drains everything in multiple sends", func(t *testing.T) {
		t.Parallel()

		sender := &drainTestSender{}
		sb := New(sender,
			WithMaxSendSizeBytes(1), // one line per send
			WithMaxDrainSends(5),
			WithDrainSendWait(time.Nanosecond),
		)
		writeLines(t, sb, "0", "1", "2")

		require.NoError(t, sb.Drain(t.Context()))

		require.Equal(t, 3, sender.sendCount())
		require.Equal(t, "012", sender.receivedData())
		require.Empty(t, sb.logs)
		require.Zero(t, sb.size)
	})

	t.Run("calls sender once when one send suffices", func(t *testing.T) {
		t.Parallel()

		sender := &drainTestSender{}
		sb := New(sender,
			WithMaxSendSizeBytes(1000), // everything fits in a single send
			WithMaxDrainSends(5),
			WithDrainSendWait(time.Microsecond),
		)
		writeLines(t, sb, "0", "1", "2")

		require.NoError(t, sb.Drain(t.Context()))

		require.Equal(t, 1, sender.sendCount())
		require.Equal(t, "012", sender.receivedData())
		require.Empty(t, sb.logs)
		require.Zero(t, sb.size)
	})

	t.Run("errors after exceeding max drain sends", func(t *testing.T) {
		t.Parallel()

		sender := &drainTestSender{}
		sb := New(sender,
			WithMaxSendSizeBytes(1), // one line per send, so 5 lines need 5 sends
			WithMaxDrainSends(2),
			WithDrainSendWait(time.Microsecond),
		)
		writeLines(t, sb, "0", "1", "2", "3", "4")

		err := sb.Drain(t.Context())

		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to drain sendbuffer after the maximum")
		require.Equal(t, 2, sender.sendCount())
		require.Len(t, sb.logs, 3) // only 2 of 5 drained
	})

	t.Run("aborts and preserves data on send error", func(t *testing.T) {
		t.Parallel()

		sender := &drainTestSender{err: errors.New("boom")}
		sb := New(sender,
			WithMaxSendSizeBytes(1),
			WithMaxDrainSends(5),
			WithDrainSendWait(time.Microsecond),
		)
		writeLines(t, sb, "0", "1", "2")

		err := sb.Drain(t.Context())

		require.Error(t, err)
		require.Contains(t, err.Error(), "draining sendbuffer failed on send and purge")
		require.Equal(t, 1, sender.sendCount())
		require.Len(t, sb.logs, 3) // nothing deleted on send failure
		require.Equal(t, 3, sb.size)
	})

	t.Run("stops and returns when context is cancelled", func(t *testing.T) {
		t.Parallel()

		sender := &drainTestSender{}
		sb := New(sender,
			WithMaxSendSizeBytes(1),
			WithMaxDrainSends(5),
			WithDrainSendWait(10*time.Second), // long, so context wins
		)
		writeLines(t, sb, "0", "1", "2")

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := sb.Drain(ctx)

		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, 1, sender.sendCount()) // one send, then bailed on the wait
		require.Len(t, sb.logs, 2)
	})

	t.Run("waits drainSendWait between sends", func(t *testing.T) {
		t.Parallel()

		sender := &drainTestSender{}
		sb := New(sender,
			WithMaxSendSizeBytes(1), // one line per send, so a wait is required between them
			WithMaxDrainSends(5),
			WithDrainSendWait(2*time.Second),
		)
		writeLines(t, sb, "0", "1", "2")

		go sb.Drain(t.Context())

		// the first send happens immediately
		require.Eventually(t, func() bool {
			return sender.sendCount() == 1
		}, 2*time.Second, 5*time.Millisecond)

		// ...but the second is waiting, so nothing follows
		time.Sleep(50 * time.Millisecond)
		require.Equal(t, 1, sender.sendCount())
	})
}
