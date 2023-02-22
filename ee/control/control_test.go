package control

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

type (
	mockConsumer struct {
		updates int
	}
	mockSubscriber struct {
		pings int
	}
	mockGetSet struct {
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

func (ms *mockGetSet) Get(key []byte) (value []byte, err error) {
	return []byte(ms.keyValues[string(key)]), nil
}

func (ms *mockGetSet) Set(key, value []byte) error {
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
			cs := New(log.NewNopLogger(), context.Background(), data, controlOpts...)
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
			cs := New(log.NewNopLogger(), context.Background(), data, controlOpts...)
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
			cs := New(log.NewNopLogger(), context.Background(), data, controlOpts...)
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
			cs := New(log.NewNopLogger(), context.Background(), data, controlOpts...)
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

			getset := &mockGetSet{keyValues: make(map[string]string)}

			// Make several instances of control service
			for j := 0; j < tt.instances; j++ {
				data := &TestClient{tt.subsystems, tt.hashData}
				controlOpts := []Option{WithGetterSetter(getset)}

				cs := New(log.NewNopLogger(), context.Background(), data, controlOpts...)
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

type testConsumer struct {
	calls []io.Reader
}

func newTestConsumer() *testConsumer {
	tc := testConsumer{
		calls: make([]io.Reader, 0),
	}
	return &tc
}

func (t *testConsumer) Update(data io.Reader) error {
	t.calls = append(t.calls, data)
	return nil
}

func TestControlServerIntegration(t *testing.T) {
	t.Parallel()

	if os.Getenv("INTEGRATION_TEST") != "1" {
		t.Skip("environment not set up for integration test")
	}

	// Setup
	logger := log.NewNopLogger()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := bbolt.Open(filepath.Join(dir, "db"), 0600, nil)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	// Set up local keys
	agent.SetupKeys(logger, db)

	// Set up client
	clientOpts := []HTTPClientOption{
		WithInsecureSkipVerify(),
		WithDisableTLS(),
	}
	client, err := NewControlHTTPClient(logger, "localhost:3000", http.DefaultClient, clientOpts...)
	require.NoError(t, err, "could not create new control client")

	// Set up control service
	controlOpts := []Option{
		WithRequestInterval(1 * time.Minute),
	}
	service := New(logger, ctx, client, controlOpts...)

	// Register a fake desktop_notifier
	testNotificationConsumer := newTestConsumer()
	service.RegisterConsumer("desktop_notifier", testNotificationConsumer)

	err = service.Fetch()
	require.NoError(t, err, "unexpected error on fetch")

	require.Equal(t, 1, len(testNotificationConsumer.calls))
}
