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
		updates  int
		updateFn func(mc *mockConsumer)
	}
	mockSubscriber struct {
		pings  int
		pingFn func(ms *mockSubscriber)
	}
)

func (mc *mockConsumer) Update(io.Reader) {
	if mc.updateFn != nil {
		mc.updateFn(mc)
	}
}
func (ms *mockSubscriber) Ping() {
	if ms.pingFn != nil {
		ms.pingFn(ms)
	}
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
		name      string
		subsystem string
		c         *mockConsumer
		s         []*mockSubscriber
	}{
		{
			name:      "one consumer, two subscribers",
			subsystem: "desktop",
			c: &mockConsumer{
				updateFn: func(mc *mockConsumer) {
					mc.updates++
				},
			},
			s: []*mockSubscriber{
				{
					pingFn: func(ms *mockSubscriber) {
						ms.pings++
					},
				},
				{
					pingFn: func(ms *mockSubscriber) {
						ms.pings++
					},
				},
			},
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
			assert.Equal(t, tt.c.updates, 1)

			// Expect each subscriber to have gotten exactly one ping
			for _, s := range tt.s {
				assert.Equal(t, s.pings, 1)
			}
		})
	}
}
