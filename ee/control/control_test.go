package control

import (
	"context"
	"errors"
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/kolide/launcher/ee/agent/flags"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/knapsack"
	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	typesMocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/ee/control/consumers/keyvalueconsumer"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type (
	mockConsumer struct {
		updates   int
		updateErr error
	}
	mockSubscriber struct {
		pings int
	}
	mockStore struct {
		keyValues map[string]string
	}
)

func (mc *mockConsumer) Update(io.Reader) error {
	mc.updates++
	return mc.updateErr
}

func (ms *mockSubscriber) Ping() {
	ms.pings++
}

func (ms *mockStore) Get(key []byte) (value []byte, err error) {
	return []byte(ms.keyValues[string(key)]), nil
}

func (ms *mockStore) Set(key, value []byte) error {
	ms.keyValues[string(key)] = string(value)
	return nil
}

type nopDataProvider struct{}

func (dp nopDataProvider) GetConfig(_ context.Context) (io.Reader, error) {
	return nil, nil
}

func (dp nopDataProvider) GetSubsystemData(_ context.Context, hash string) (io.Reader, error) {
	return nil, nil
}

func (dp nopDataProvider) SendMessage(_ context.Context, method string, params interface{}) error {
	return nil
}

func TestControlServiceRegisterConsumer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		subsystem string
		c         *mockConsumer
	}{
		{
			name:      "empty subsystem",
			subsystem: "",
			c:         &mockConsumer{},
		},
		{
			name:      "happy path",
			subsystem: "desktop",
			c:         &mockConsumer{},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockKnapsack := typesMocks.NewKnapsack(t)
			mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.ControlRequestInterval)
			mockKnapsack.On("ControlRequestInterval").Return(60 * time.Second)
			mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())

			data := nopDataProvider{}
			controlOpts := []Option{}
			cs := New(mockKnapsack, data, controlOpts...)
			err := cs.RegisterConsumer(tt.subsystem, tt.c)
			require.NoError(t, err)
		})
	}
}

func TestControlServiceRegisterConsumerMultiple(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		subsystem string
		c         *mockConsumer
	}{
		{
			name:      "registered twice",
			subsystem: "desktop",
			c:         &mockConsumer{},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockKnapsack := typesMocks.NewKnapsack(t)
			mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.ControlRequestInterval)
			mockKnapsack.On("ControlRequestInterval").Return(60 * time.Second)
			mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())

			data := nopDataProvider{}
			controlOpts := []Option{}
			cs := New(mockKnapsack, data, controlOpts...)
			err := cs.RegisterConsumer(tt.subsystem, tt.c)
			require.NoError(t, err)
			err = cs.RegisterConsumer(tt.subsystem, tt.c)
			require.Error(t, err)
		})
	}
}

func TestControlServiceUpdate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		subsystem       string
		c               *mockConsumer
		s               []*mockSubscriber
		expectedUpdates int
	}{
		{
			name:            "one consumer, two subscribers",
			subsystem:       "desktop",
			expectedUpdates: 1,
			c:               &mockConsumer{},
			s: []*mockSubscriber{
				{},
				{},
			},
		},
		{
			name:            "one consumer, no subscribers",
			subsystem:       "desktop",
			expectedUpdates: 1,
			c:               &mockConsumer{},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockKnapsack := typesMocks.NewKnapsack(t)
			mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.ControlRequestInterval)
			mockKnapsack.On("ControlRequestInterval").Return(60 * time.Second)
			mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())

			data := nopDataProvider{}
			controlOpts := []Option{}
			cs := New(mockKnapsack, data, controlOpts...)
			err := cs.RegisterConsumer(tt.subsystem, tt.c)
			require.NoError(t, err)
			for _, ss := range tt.s {
				cs.RegisterSubscriber(tt.subsystem, ss)
			}

			err = cs.update(context.TODO(), tt.subsystem, nil)
			require.NoError(t, err)

			// Expect consumer to have gotten exactly one update
			assert.Equal(t, tt.expectedUpdates, tt.c.updates)

			// Expect each subscriber to have gotten exactly one ping
			for _, s := range tt.s {
				assert.Equal(t, tt.expectedUpdates, s.pings)
			}
		})
	}
}

func TestControlServiceUpdateErr(t *testing.T) {
	t.Parallel()

	// Create mock consumer that returns error on update
	errConsumer := &mockConsumer{
		updateErr: errors.New("simulated update failure"),
	}

	mockKnapsack := typesMocks.NewKnapsack(t)
	mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.ControlRequestInterval)
	mockKnapsack.On("ControlRequestInterval").Return(60 * time.Second)
	mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())

	// Set up test data with known hash
	subsystems := map[string]string{"actions": "abc123"}
	hashData := map[string]any{"abc123": "test-data"}
	data, _ := NewControlTestClient(subsystems, hashData)

	// Create control service and register error-producing consumer
	store := &mockStore{keyValues: make(map[string]string)}
	controlOpts := []Option{WithStore(store)}
	cs := New(mockKnapsack, data, controlOpts...)
	err := cs.RegisterConsumer("actions", errConsumer)
	require.NoError(t, err)

	// Trigger fetch which should cause update error
	err = cs.Fetch(context.TODO())
	require.NoError(t, err)

	// Verify error consumer was called
	assert.Equal(t, 1, errConsumer.updates)

	// Verify hash was not recorded due to error
	val, err := store.Get([]byte("actions"))
	require.NoError(t, err)
	assert.Empty(t, string(val))
}

func TestControlServiceRetryAfterUpdateErr(t *testing.T) {
	t.Parallel()

	errConsumer := &mockConsumer{
		updateErr: errors.New("simulated update failure"),
	}

	mockKnapsack := typesMocks.NewKnapsack(t)
	mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.ControlRequestInterval)
	mockKnapsack.On("ControlRequestInterval").Return(60 * time.Second)
	mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())

	// First fetch data
	subsystems := map[string]string{"actions": "abc123"}
	hashData := map[string]any{"abc123": "test-data-1"}
	data, _ := NewControlTestClient(subsystems, hashData)

	store := &mockStore{keyValues: make(map[string]string)}
	controlOpts := []Option{WithStore(store)}
	cs := New(mockKnapsack, data, controlOpts...)

	err := cs.RegisterConsumer("actions", errConsumer)
	require.NoError(t, err)

	// First fetch - should fail and not store hash
	err = cs.Fetch(context.TODO())
	require.NoError(t, err)
	assert.Equal(t, 1, errConsumer.updates)

	// Create new test client with updated hash
	subsystems = map[string]string{"actions": "def456"}
	hashData = map[string]any{"def456": "test-data-2"}
	newData, _ := NewControlTestClient(subsystems, hashData)
	cs.fetcher = newData

	// Second fetch with new hash - should succeed
	errConsumer.updateErr = nil
	err = cs.Fetch(context.TODO())
	require.NoError(t, err)
	assert.Equal(t, 2, errConsumer.updates)

	// Verify final hash was recorded
	val, err := store.Get([]byte("actions"))
	require.NoError(t, err)
	assert.Equal(t, "def456", string(val))
}

func TestControlServiceFetch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		hashData        map[string]any
		subsystems      map[string]string
		subsystem       string
		c               *mockConsumer
		s               []*mockSubscriber
		expectedUpdates int
		fetches         int
	}{
		{
			name:            "one consumer, two subscribers",
			subsystem:       "desktop",
			subsystems:      map[string]string{"desktop": "502a42f0"},
			hashData:        map[string]any{"502a42f0": "status"},
			expectedUpdates: 1,
			fetches:         3,
			c:               &mockConsumer{},
			s: []*mockSubscriber{
				{},
				{},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockKnapsack := typesMocks.NewKnapsack(t)
			mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.ControlRequestInterval)
			mockKnapsack.On("ControlRequestInterval").Return(60 * time.Second)
			mockKnapsack.On("ForceControlSubsystems").Return(false)
			mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())

			data, _ := NewControlTestClient(tt.subsystems, tt.hashData)
			controlOpts := []Option{}
			cs := New(mockKnapsack, data, controlOpts...)
			err := cs.RegisterConsumer(tt.subsystem, tt.c)
			require.NoError(t, err)
			for _, ss := range tt.s {
				cs.RegisterSubscriber(tt.subsystem, ss)
			}

			// Repeat fetches to verify no changes
			for i := 0; i < tt.fetches; i++ {
				err = cs.Fetch(context.TODO())
				require.NoError(t, err)

				// Expect consumer to have gotten exactly one update
				assert.Equal(t, tt.expectedUpdates, tt.c.updates)

				// Expect each subscriber to have gotten exactly one ping
				for _, s := range tt.s {
					assert.Equal(t, tt.expectedUpdates, s.pings)
				}
			}
		})
	}
}

func TestControlServiceFetch_IgnoresUnknownSubsystems(t *testing.T) {
	t.Parallel()

	mockKnapsack := typesMocks.NewKnapsack(t)
	mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.ControlRequestInterval)
	mockKnapsack.On("ControlRequestInterval").Return(60 * time.Second)
	mockKnapsack.On("ForceControlSubsystems").Return(false)
	mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())

	unknownSubsystemHash := "602a42f1"
	subsystems := map[string]string{"desktop": "502a42f0", "unknown": unknownSubsystemHash}
	hashData := map[string]any{"502a42f0": "status"}

	// The test client will panic if it receives a request for the `unknown` subsystem hash,
	// validating that we don't process data for the `unknown` subsystem.
	data, _ := NewControlTestClient(subsystems, hashData)
	controlOpts := []Option{}
	cs := New(mockKnapsack, data, controlOpts...)

	// Register consumer/subscriber for `desktop` subsystem only
	desktopConsumer := &mockConsumer{}
	err := cs.RegisterConsumer("desktop", desktopConsumer)
	require.NoError(t, err)
	desktopSubscriber := &mockSubscriber{}
	cs.RegisterSubscriber("desktop", desktopSubscriber)

	// Run fetch for the first time
	require.NoError(t, cs.Fetch(context.TODO()))

	// Expect desktop consumer to have gotten exactly one update
	assert.Equal(t, 1, desktopConsumer.updates)

	// Expect desktop subscriber to have gotten exactly one ping
	assert.Equal(t, 1, desktopSubscriber.pings)

	// Expect no requests to unknown hash
	require.NotContains(t, data.hashRequestCounts, unknownSubsystemHash)

	// Run fetch again. This time, data should be cached, so there should be no updates or pings.
	require.NoError(t, cs.Fetch(context.TODO()))
	assert.Equal(t, 1, desktopConsumer.updates)
	assert.Equal(t, 1, desktopSubscriber.pings)

	// Expect no requests to unknown hash
	require.NotContains(t, data.hashRequestCounts, unknownSubsystemHash)
}

func TestControlServiceFetch_WithControlRequestIntervalUpdate(t *testing.T) {
	t.Parallel()

	// We want an actual, minimal test knapsack rather than a mock
	// to ensure that we get a call to `FlagsChanged` when the
	// `ControlRequestInterval` flag changes during a `Fetch`.
	nopMultislogger := multislogger.New()
	db := storageci.SetupDB(t)
	stores, err := storageci.MakeStores(t, nopMultislogger.Logger, db)
	require.NoError(t, err)
	flagController := flags.NewFlagController(nopMultislogger.Logger, stores[storage.AgentFlagsStore])
	startingRequestInterval := 5 * time.Second // this is the minimum value
	flagController.SetControlRequestInterval(startingRequestInterval)
	testKnapsack := knapsack.New(stores, flagController, db, nopMultislogger, nopMultislogger)

	// Init the test client
	expectedInterval := 6 * time.Second
	expectedIntervalInNanoseconds := expectedInterval.Nanoseconds()
	subsystemMap := map[string]string{"agent_flags": "302a42f3"}
	hashData := map[string]any{"302a42f3": map[string]string{
		"control_request_interval": strconv.FormatInt(expectedIntervalInNanoseconds, 10),
	}}
	client, err := NewControlTestClient(subsystemMap, hashData)
	require.NoError(t, err)

	// Init the service and register consumer
	controlOpts := []Option{
		WithStore(testKnapsack.ControlStore()),
	}
	service := New(testKnapsack, client, controlOpts...)
	require.NoError(t, service.RegisterConsumer("agent_flags", keyvalueconsumer.New(flagController)))

	// Start running and wait at least a couple request intervals for the change to be applied
	go service.Start(context.TODO())
	time.Sleep(2 * startingRequestInterval)

	// Stop the service
	service.Interrupt(errors.New("test error"))

	// Confirm request interval was updated as expected
	require.Equal(t, expectedInterval, service.readRequestInterval())
}

func TestControlServicePersistLastFetched(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		hashData        map[string]any
		subsystems      map[string]string
		subsystem       string
		c               *mockConsumer
		expectedUpdates int
		instances       int
	}{
		{
			name:            "one consumer",
			subsystem:       "desktop",
			subsystems:      map[string]string{"desktop": "502a42f0"},
			hashData:        map[string]any{"502a42f0": "status"},
			expectedUpdates: 1,
			instances:       3,
			c:               &mockConsumer{},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := &mockStore{keyValues: make(map[string]string)}

			// Make several instances of control service
			for j := 0; j < tt.instances; j++ {
				data, _ := NewControlTestClient(tt.subsystems, tt.hashData)
				controlOpts := []Option{WithStore(store)}

				mockKnapsack := typesMocks.NewKnapsack(t)
				mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.ControlRequestInterval)
				mockKnapsack.On("ControlRequestInterval").Return(60 * time.Second)
				mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())

				if j >= tt.expectedUpdates {
					// ForceControlSubsystems is only called when no update would normally be triggered
					mockKnapsack.On("ForceControlSubsystems").Return(false)
				}

				cs := New(mockKnapsack, data, controlOpts...)
				err := cs.RegisterConsumer(tt.subsystem, tt.c)
				require.NoError(t, err)

				err = cs.Fetch(context.TODO())
				require.NoError(t, err)
			}

			// Expect consumer to have gotten exactly one update
			assert.Equal(t, tt.expectedUpdates, tt.c.updates)
		})
	}
}

func Test_knownSubsystem(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName   string
		subsystemName  string
		consumers      []string
		subscribers    []string
		subsystemKnown bool
	}{
		{
			testCaseName:   "subsystem in consumers",
			subsystemName:  "subsystem_to_query",
			consumers:      []string{"subsystem_to_query", "some_other_subsystem"},
			subscribers:    []string{"some_other_subsystem", "a_third_subsystem"},
			subsystemKnown: true,
		},
		{
			testCaseName:   "subsystem in subscribers",
			subsystemName:  "subsystem_to_query",
			consumers:      []string{"some_other_subsystem"},
			subscribers:    []string{"some_other_subsystem", "subsystem_to_query"},
			subsystemKnown: true,
		},
		{
			testCaseName:   "subsystem in consumers and subscribers",
			subsystemName:  "subsystem_to_query",
			consumers:      []string{"subsystem_to_query", "some_other_subsystem"},
			subscribers:    []string{"some_other_subsystem", "subsystem_to_query"},
			subsystemKnown: true,
		},
		{
			testCaseName:   "subsystem is unknown",
			subsystemName:  "subsystem_to_query",
			consumers:      []string{"some_other_subsystem", "another_test_subsystem"},
			subscribers:    []string{"some_other_subsystem"},
			subsystemKnown: false,
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			mockKnapsack := typesMocks.NewKnapsack(t)
			mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.ControlRequestInterval)
			mockKnapsack.On("ControlRequestInterval").Return(60 * time.Second)
			mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())

			controlOpts := []Option{}
			cs := New(mockKnapsack, nil, controlOpts...)

			// Register known consumers and subscribers
			for _, c := range tt.consumers {
				require.NoError(t, cs.RegisterConsumer(c, &mockConsumer{}))
			}
			for _, s := range tt.subscribers {
				cs.RegisterSubscriber(s, &mockSubscriber{})
			}

			known := cs.knownSubsystem(tt.subsystemName)
			require.Equal(t, tt.subsystemKnown, known)
		})
	}
}

func TestInterrupt_Multiple(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	k := typesMocks.NewKnapsack(t)
	k.On("ControlRequestInterval").Return(24 * time.Hour)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	k.On("Slogger").Return(multislogger.NewNopLogger())
	data := &TestClient{}
	control := New(k, data)

	go control.ExecuteWithContext(ctx)

	time.Sleep(3 * time.Second)
	control.Interrupt(errors.New("test error"))

	// Confirm we can call Interrupt multiple times without blocking
	interruptComplete := make(chan struct{})
	expectedInterrupts := 3
	for i := 0; i < expectedInterrupts; i += 1 {
		go func() {
			control.Interrupt(nil)
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
			t.Errorf("could not call interrupt multiple times and return within 5 seconds -- received %d interrupts before timeout", receivedInterrupts)
			t.FailNow()
		}
	}

	require.Equal(t, expectedInterrupts, receivedInterrupts)
}
