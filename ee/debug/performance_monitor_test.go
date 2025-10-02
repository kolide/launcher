package debug

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/ee/performance"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
)

func TestInterrupt_Multiple(t *testing.T) {
	t.Parallel()

	testKnapsack := typesmocks.NewKnapsack(t)
	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	testKnapsack.On("Slogger").Return(slogger)

	p := NewPerformanceMonitor(testKnapsack)

	// Start and then interrupt
	go p.Execute()
	time.Sleep(3 * time.Second)
	interruptStart := time.Now()
	p.Interrupt(errors.New("test error"))

	// Confirm we can call Interrupt multiple times without blocking
	interruptComplete := make(chan struct{})
	expectedInterrupts := 3
	for i := 0; i < expectedInterrupts; i += 1 {
		go func() {
			p.Interrupt(nil)
			interruptComplete <- struct{}{}
		}()
	}

	receivedInterrupts := 0
	for {
		if receivedInterrupts >= expectedInterrupts {
			break
		}

		select {
		case <-interruptComplete:
			receivedInterrupts += 1
			continue
		case <-time.After(5 * time.Second):
			t.Errorf("could not call interrupt multiple times and return within 5 seconds -- interrupted at %s, received %d interrupts before timeout; logs: \n%s\n", interruptStart.String(), receivedInterrupts, logBytes.String())
			t.FailNow()
		}
	}

	require.Equal(t, expectedInterrupts, receivedInterrupts)
}

func Test_checkPerformance_notEnabled(t *testing.T) {
	t.Parallel()

	testKnapsack := typesmocks.NewKnapsack(t)
	testKnapsack.On("Slogger").Return(multislogger.NewNopLogger())
	testKnapsack.On("PerformanceMonitoringEnabled").Return(false)

	p := NewPerformanceMonitor(testKnapsack)

	p.checkPerformance()

	// Expect that we did not trigger a flare
	require.Equal(t, int64(0), p.lastFlareSent.Load())
}

func Test_shouldTriggerFlareUpload(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName      string
		cpuPercent        float64
		goMemUsage        uint64
		lastSentTimestamp int64
		expectFlareUpload bool
		expectedReason    string
	}{
		{
			testCaseName:      "CPU too high, flare not triggered recently",
			cpuPercent:        cpuUsageThresholdForFlare + 5,
			goMemUsage:        golangMemUsageThresholdForFlare - 800,
			lastSentTimestamp: time.Now().Unix() - minimumFlareResendIntervalSeconds - 5000,
			expectFlareUpload: true,
			expectedReason:    "high CPU usage",
		},
		{
			testCaseName:      "memory too high, flare not triggered recently",
			cpuPercent:        cpuUsageThresholdForFlare - 3,
			goMemUsage:        golangMemUsageThresholdForFlare + 1500,
			lastSentTimestamp: time.Now().Unix() - minimumFlareResendIntervalSeconds - 1000,
			expectFlareUpload: true,
			expectedReason:    "high Golang memory usage",
		},
		{
			testCaseName:      "CPU and memory too high, flare not triggered recently",
			cpuPercent:        cpuUsageThresholdForFlare + 4,
			goMemUsage:        golangMemUsageThresholdForFlare + 6000,
			lastSentTimestamp: time.Now().Unix() - minimumFlareResendIntervalSeconds - 600,
			expectFlareUpload: true,
			expectedReason:    "high Golang memory and CPU usage",
		},
		{
			testCaseName:      "CPU and memory within allowable range, flare not triggered recently",
			cpuPercent:        cpuUsageThresholdForFlare - 2,
			goMemUsage:        golangMemUsageThresholdForFlare - 4000,
			lastSentTimestamp: time.Now().Unix() - minimumFlareResendIntervalSeconds - 300,
			expectFlareUpload: false,
		},
		{
			testCaseName:      "CPU too high, flare triggered recently",
			cpuPercent:        cpuUsageThresholdForFlare + 6,
			goMemUsage:        golangMemUsageThresholdForFlare - 1000,
			lastSentTimestamp: time.Now().Unix() - (minimumFlareResendIntervalSeconds - 5000),
			expectFlareUpload: false,
		},
		{
			testCaseName:      "memory too high, flare triggered recently",
			cpuPercent:        cpuUsageThresholdForFlare - 1,
			goMemUsage:        golangMemUsageThresholdForFlare + 2000,
			lastSentTimestamp: time.Now().Unix() - (minimumFlareResendIntervalSeconds - 1000),
			expectFlareUpload: false,
		},
		{
			testCaseName:      "CPU and memory too high, flare triggered recently",
			cpuPercent:        cpuUsageThresholdForFlare + 7,
			goMemUsage:        golangMemUsageThresholdForFlare + 3000,
			lastSentTimestamp: time.Now().Unix() - (minimumFlareResendIntervalSeconds - 600),
			expectFlareUpload: false,
		},
		{
			testCaseName:      "CPU and memory within allowable range, flare triggered recently",
			cpuPercent:        cpuUsageThresholdForFlare - 2,
			goMemUsage:        golangMemUsageThresholdForFlare - 900,
			lastSentTimestamp: time.Now().Unix() - (minimumFlareResendIntervalSeconds - 300),
			expectFlareUpload: false,
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			testKnapsack := typesmocks.NewKnapsack(t)
			testKnapsack.On("Slogger").Return(multislogger.NewNopLogger())

			p := NewPerformanceMonitor(testKnapsack)
			p.lastFlareSent.Store(tt.lastSentTimestamp)

			testStats := &performance.PerformanceStats{
				CPUPercent: tt.cpuPercent,
				MemInfo: &performance.MemInfo{
					GoMemUsage: tt.goMemUsage,
				},
			}

			performUpload, uploadReason := p.shouldTriggerFlareUpload(t.Context(), testStats)

			require.Equal(t, tt.expectFlareUpload, performUpload)
			require.Equal(t, tt.expectedReason, uploadReason)
		})
	}
}
