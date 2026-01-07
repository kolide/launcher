//go:build windows

package watchdog

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// mockTaskManager is a testing helper which adheres to the taskManager interface.
// this allows us to unit test a bunch of the Controller logic without actually
// attempting task removal or installation
type mockTaskManager struct {
	taskInstallState atomic.Int32
	slogger          *slog.Logger
}

func (mtm *mockTaskManager) currentInstallState() installState {
	return mtm.taskInstallState.Load()
}

func (mtm *mockTaskManager) setCurrentInstallState(state installState) {
	mtm.taskInstallState.Store(state)
}

func (mtm *mockTaskManager) installTask() error {
	mtm.slogger.Log(context.TODO(), slog.LevelDebug, "install task called")
	mtm.setCurrentInstallState(installStateInstalled)
	return nil
}

func (mtm *mockTaskManager) removeTask() error {
	mtm.slogger.Log(context.TODO(), slog.LevelDebug, "remove task called")
	mtm.setCurrentInstallState(installStateRemoved)
	return nil
}

func TestInterrupt_Multiple(t *testing.T) {
	t.Parallel()
	tempRootDir := t.TempDir()
	var logBytes threadsafebuffer.ThreadSafeBuffer
	testSlogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("RootDirectory").Return(tempRootDir)
	mockKnapsack.On("Slogger").Return(testSlogger)
	mockKnapsack.On("Identifier").Return("kolide-k2").Maybe()
	mockKnapsack.On("KolideServerURL").Return("k2device.kolide.com")
	mockKnapsack.On("LauncherWatchdogDisabled").Return(false).Maybe()

	controller, _ := NewController(t.Context(), mockKnapsack, "")

	require.True(t, controller.shouldManageWatchdog(), "could not manage watchdog, running in admin mode?")

	// Let the handler run for a bit
	go controller.Run()
	time.Sleep(3 * time.Second)
	interruptStart := time.Now()
	controller.Interrupt(errors.New("test error"))

	// Confirm we can call Interrupt multiple times without blocking
	interruptComplete := make(chan struct{})
	expectedInterrupts := 3
	for i := 0; i < expectedInterrupts; i += 1 {
		go func() {
			controller.Interrupt(nil)
			interruptComplete <- struct{}{}
		}()
	}

	receivedInterrupts := 0
	for receivedInterrupts < expectedInterrupts {
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

func TestFlagsChanged(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		testCaseName        string
		watchdogDisabled    bool
		initialInstallState installState
		finalInstallState   installState
		// we use expectRemoval/expectInstallation to assert the removal actually happened
		// (wasn't bypassed due to cached installState values). this is checked via slogger bytes
		expectRemoval      bool
		expectInstallation bool
	}{
		{
			testCaseName:        "triggering removal after already being installed",
			watchdogDisabled:    true,
			initialInstallState: installStateInstalled,
			finalInstallState:   installStateRemoved,
			expectRemoval:       true,
			expectInstallation:  false,
		},
		{
			testCaseName:        "triggering install after already being removed",
			watchdogDisabled:    false,
			initialInstallState: installStateRemoved,
			finalInstallState:   installStateInstalled,
			expectRemoval:       false,
			expectInstallation:  true,
		},
		{
			testCaseName:        "requesting install after already being installed",
			watchdogDisabled:    false,
			initialInstallState: installStateInstalled,
			finalInstallState:   installStateInstalled,
			expectRemoval:       false,
			expectInstallation:  false,
		},
		{
			testCaseName:        "requesting removal after already being removed",
			watchdogDisabled:    true,
			initialInstallState: installStateRemoved,
			finalInstallState:   installStateRemoved,
			expectRemoval:       false,
			expectInstallation:  false,
		},
	} {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()
			var logBytes threadsafebuffer.ThreadSafeBuffer
			testSlogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			}))
			mockKnapsack := typesmocks.NewKnapsack(t)
			mockKnapsack.On("Identifier").Return("test-identifier").Maybe()
			mockKnapsack.On("KolideServerURL").Return("k2device.kolide.com")
			mockKnapsack.On("LauncherWatchdogDisabled").Return(tt.watchdogDisabled).Maybe()

			controller := &WatchdogController{
				slogger:     testSlogger,
				knapsack:    mockKnapsack,
				interrupt:   make(chan struct{}, 1),
				taskManager: &mockTaskManager{slogger: testSlogger},
			}

			require.True(t, controller.shouldManageWatchdog(), "could not manage watchdog, running in admin mode?")

			controller.taskManager.setCurrentInstallState(tt.initialInstallState)
			controller.FlagsChanged(t.Context(), keys.LauncherWatchdogDisabled)
			require.Equal(t, tt.finalInstallState, controller.taskManager.currentInstallState())
			if tt.expectInstallation {
				require.Contains(t, logBytes.String(), "install task called")
			} else {
				require.NotContains(t, logBytes.String(), "install task called")
			}

			if tt.expectRemoval {
				require.Contains(t, logBytes.String(), "remove task called")
			} else {
				require.NotContains(t, logBytes.String(), "remove task called")
			}
		})
	}
}

func TestPublishLogs(t *testing.T) {
	t.Parallel()
	testSlogger := multislogger.NewNopLogger()
	tempRootDir := t.TempDir()

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("KolideServerURL").Return("k2device.kolide.com")
	mockKnapsack.On("LauncherWatchdogDisabled").Return(false)
	mockKnapsack.On("RootDirectory").Return(tempRootDir)

	t.Run("successfully publishes and deletes logs", func(t *testing.T) {
		t.Parallel()

		mockPublisher := typesmocks.NewTimestampedIteratorDeleterAppenderCloser(t)
		mockPublisher.On("ForEach", mock.Anything).Run(func(args mock.Arguments) {
			fn := args.Get(0).(func(int64, int64, []byte) error)
			fn(1, 1000, []byte(`{"msg":"test log 1"}`))
			fn(2, 2000, []byte(`{"msg":"test log 2"}`))
		}).Return(nil)
		mockPublisher.On("DeleteRows", int64(1), int64(2)).Return(nil)

		controller := &WatchdogController{
			slogger:      testSlogger,
			knapsack:     mockKnapsack,
			interrupt:    make(chan struct{}, 1),
			logPublisher: mockPublisher,
			taskManager:  &mockTaskManager{slogger: testSlogger},
		}

		controller.publishLogs(t.Context())

		mockPublisher.AssertCalled(t, "DeleteRows", int64(1), int64(2))
	})

	t.Run("no logs to publish", func(t *testing.T) {
		t.Parallel()

		mockPublisher := typesmocks.NewTimestampedIteratorDeleterAppenderCloser(t)
		mockPublisher.On("ForEach", mock.Anything).Return(nil)

		controller := &WatchdogController{
			slogger:      testSlogger,
			knapsack:     mockKnapsack,
			interrupt:    make(chan struct{}, 1),
			logPublisher: mockPublisher,
			taskManager:  &mockTaskManager{slogger: testSlogger},
		}

		controller.publishLogs(t.Context())

		mockPublisher.AssertNotCalled(t, "DeleteRows", "delete rows should not be called for no logs")
	})

	t.Run("ForEach error", func(t *testing.T) {
		t.Parallel()

		mockPublisher := typesmocks.NewTimestampedIteratorDeleterAppenderCloser(t)
		mockPublisher.On("ForEach", mock.Anything).Return(errors.New("some err"))
		mockPublisher.On("Close").Return(errors.New("some err"))

		controller := &WatchdogController{
			slogger:      testSlogger,
			knapsack:     mockKnapsack,
			interrupt:    make(chan struct{}, 1),
			logPublisher: mockPublisher,
			taskManager:  &mockTaskManager{slogger: testSlogger},
		}

		controller.publishLogs(t.Context())

		mockPublisher.AssertCalled(t, "Close")
		require.NotEqual(t, mockPublisher, controller.logPublisher, "logPublisher should have been replaced with a new publisher")
		require.NoError(t, controller.logPublisher.Close())
	})
}
