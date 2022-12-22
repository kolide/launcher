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
		updateFn func()
	}
	mockSubscriber struct {
		pingFn func()
	}
)

func (mc *mockConsumer) Update(io.Reader) {
	if mc.updateFn != nil {
		mc.updateFn()
	}
}
func (mc *mockSubscriber) Ping() {
	if mc.pingFn != nil {
		mc.pingFn()
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
		c         consumer
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
		c         consumer
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

	var updateCount int
	var pingCount int
	tests := []struct {
		name      string
		subsystem string
		c         consumer
		s         []subscriber
	}{
		{
			name:      "one consumer, two subscribers",
			subsystem: "desktop",
			c: &mockConsumer{
				updateFn: func() {
					updateCount++
				},
			},
			s: []subscriber{
				&mockSubscriber{
					pingFn: func() {
						pingCount++
					},
				},
				&mockSubscriber{
					pingFn: func() {
						pingCount++
					},
				},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			updateCount, pingCount = 0, 0
			data := nopDataProvider{}
			controlOpts := []Option{}
			cs := New(log.NewNopLogger(), data, controlOpts...)
			err := cs.RegisterConsumer(tt.subsystem, tt.c)
			require.NoError(t, err)
			for _, ss := range tt.s {
				cs.RegisterSubscriber(tt.subsystem, ss)
			}

			cs.update(tt.subsystem, nil)

			assert.Equal(t, updateCount, 1)
			assert.Equal(t, pingCount, 2)
		})
	}
}
