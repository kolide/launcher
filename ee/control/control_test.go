package control

import (
	"io"
	"testing"

	"github.com/go-kit/kit/log"
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
)

func (mc *mockConsumer) Update(io.Reader) {
	mc.updates++
}
func (ms *mockSubscriber) Ping() {
	ms.pings++
}

type nopDataProvider struct{}

func (dp nopDataProvider) Get(hash string) (data io.Reader, err error) {
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

			cs.update(tt.subsystem, nil)

			// Expect consumer to have gotten exactly one update
			assert.Equal(t, tt.c.updates, tt.expectedUpdates)

			// Expect each subscriber to have gotten exactly one ping
			for _, s := range tt.s {
				assert.Equal(t, s.pings, tt.expectedUpdates)
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

				// Expect consumer to have gotten exactly one update
				assert.Equal(t, tt.c.updates, tt.expectedUpdates)

				// Expect each subscriber to have gotten exactly one ping
				for _, s := range tt.s {
					assert.Equal(t, s.pings, tt.expectedUpdates)
				}
			}
		})
	}
}
