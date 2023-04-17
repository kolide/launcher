package flags

import (
	"testing"
	"time"

	"github.com/kolide/launcher/pkg/agent/flags/keys"
	"github.com/stretchr/testify/assert"
)

func TestFlagValueOverride(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		key           keys.FlagKey
		overrideValue any
		duration      time.Duration
	}{
		{
			name:          "happy path",
			key:           keys.ControlRequestInterval,
			overrideValue: 1 * time.Second,
			duration:      2 * time.Second,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			o := &Override{}
			assert.Nil(t, o.Value())

			ch := make(chan bool)
			expiredCallback := func(key keys.FlagKey) {
				ch <- true
			}

			o.Start(tt.key, tt.overrideValue, tt.duration, expiredCallback)
			assert.Equal(t, tt.overrideValue, o.Value())

			time.Sleep(tt.duration * 2)
			assert.Equal(t, tt.overrideValue, o.Value())
			assert.True(t, <-ch)
		})
	}
}

func TestFlagValueOverrideRestart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		key           keys.FlagKey
		overrideValue any
		duration      time.Duration
	}{
		{
			name:          "happy path",
			key:           keys.ControlRequestInterval,
			overrideValue: 1 * time.Second,
			duration:      2 * time.Second,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			o := &Override{}
			assert.Nil(t, o.Value())

			ch := make(chan bool)
			expiredCallback := func(key keys.FlagKey) {
				ch <- true
			}

			neverEverCallback := func(key keys.FlagKey) {
				assert.Fail(t, "Override expire callback called when it should not be")
			}

			o.Start(tt.key, tt.overrideValue, tt.duration, neverEverCallback)
			assert.Equal(t, tt.overrideValue, o.Value())

			time.Sleep(tt.duration / 2)
			o.Start(tt.key, tt.overrideValue, tt.duration, expiredCallback)
			assert.Equal(t, tt.overrideValue, o.Value())

			time.Sleep(tt.duration * 2)
			assert.Equal(t, tt.overrideValue, o.Value())
			assert.True(t, <-ch)
		})
	}
}
