package control

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/ee/control/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type (
	mockConsumer struct {
		updates int
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
	return nil
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

func (dp nopDataProvider) GetConfig() (io.Reader, error) {
	return nil, nil
}

func (dp nopDataProvider) GetSubsystemData(hash string) (io.Reader, error) {
	return nil, nil
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

			data := nopDataProvider{}
			controlOpts := []Option{}
			cs := New(log.NewNopLogger(), data, controlOpts...)
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

			data := nopDataProvider{}
			controlOpts := []Option{}
			cs := New(log.NewNopLogger(), data, controlOpts...)
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

			data := nopDataProvider{}
			controlOpts := []Option{}
			cs := New(log.NewNopLogger(), data, controlOpts...)
			err := cs.RegisterConsumer(tt.subsystem, tt.c)
			require.NoError(t, err)
			for _, ss := range tt.s {
				cs.RegisterSubscriber(tt.subsystem, ss)
			}

			err = cs.update(tt.subsystem, nil)
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

			data := &TestClient{tt.subsystems, tt.hashData}
			controlOpts := []Option{}
			cs := New(log.NewNopLogger(), data, controlOpts...)
			err := cs.RegisterConsumer(tt.subsystem, tt.c)
			require.NoError(t, err)
			for _, ss := range tt.s {
				cs.RegisterSubscriber(tt.subsystem, ss)
			}

			// Repeat fetches to verify no changes
			for i := 0; i < tt.fetches; i++ {
				err = cs.Fetch()
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
				data := &TestClient{tt.subsystems, tt.hashData}
				controlOpts := []Option{WithStore(store)}

				cs := New(log.NewNopLogger(), data, controlOpts...)
				err := cs.RegisterConsumer(tt.subsystem, tt.c)
				require.NoError(t, err)

				err = cs.Fetch()
				require.NoError(t, err)
			}

			// Expect consumer to have gotten exactly one update
			assert.Equal(t, tt.expectedUpdates, tt.c.updates)
		})
	}
}

func TestControlService_AccelerateRequestInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                                                      string
		startInterval, accelerationInterval, accelerationDuration time.Duration
	}{
		{
			name:                 "happy path",
			startInterval:        2 * time.Second,
			accelerationInterval: 250 * time.Millisecond,
			accelerationDuration: 1 * time.Second,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Overly verbose way to figure out how many fetches we expect to make it more human readable (hopefully)
			fetchOnControlServiceStart := 1
			fetchOnFirstTick := 1
			fetchOnFirstAccelerationCall := 1
			fetchOnFirstAccelerationTick := 1
			// after the first acceleration tick, we make 2 concurrent calls resetting the ticker
			fetchOnSecondAcclerationCall := 2
			// then we wait the full duration of the second acceleration call
			fetchesDuringSecondAccleration := int(tt.accelerationDuration.Milliseconds() / tt.accelerationInterval.Milliseconds())
			// then we wait for 2 ticks of the initial interval
			fetchAfterDeceleration := 2

			expectedFetches :=
				fetchOnControlServiceStart +
					fetchOnFirstTick +
					fetchOnFirstAccelerationCall +
					fetchOnFirstAccelerationTick +
					fetchOnSecondAcclerationCall +
					fetchesDuringSecondAccleration +
					fetchAfterDeceleration

			mockDataProvider := mocks.NewDataProvider(t)
			mockDataProvider.On("GetConfig").Return(nil, errors.New("test")).Times(expectedFetches)

			cs := New(log.NewNopLogger(), mockDataProvider, WithRequestInterval(tt.startInterval))

			// expect 1 fetch on start
			go cs.Start(context.Background())

			// expect 1 fetch on initial interval
			sleepWithBufDuration(tt.startInterval)

			// expect 1 fetch on acceleration request
			go require.NoError(t, cs.AccelerateRequestInterval(tt.accelerationInterval, tt.accelerationDuration))

			// expect 1 fetch during single tick of acceleration
			sleepWithBufDuration(tt.accelerationInterval)

			// expect 1 fetch on acceleration request
			go require.NoError(t, cs.AccelerateRequestInterval(tt.accelerationInterval, tt.accelerationDuration))
			// expect 1 fetch on acceleration request
			go require.NoError(t, cs.AccelerateRequestInterval(tt.accelerationInterval, tt.accelerationDuration))

			// expect (accelerationDuration / accelerationInterval) fetching during accleration duration
			sleepWithBufDuration(tt.accelerationDuration)

			// expect 2 fetches after accleration interval has ended and start interval has passed
			sleepWithBufDuration(tt.startInterval * 2)

			cs.Interrupt(nil)
		})
	}
}

// adds a little buffer to the duration then sleeps to account for time imprecision
func sleepWithBufDuration(d time.Duration) {
	time.Sleep(d + (200 * time.Millisecond))
}
