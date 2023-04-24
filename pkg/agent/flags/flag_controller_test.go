package flags

import (
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/agent/flags/keys"
	"github.com/kolide/launcher/pkg/agent/storage"
	storageci "github.com/kolide/launcher/pkg/agent/storage/ci"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/agent/types/mocks"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestControllerBoolFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		cmdLineOpts     *launcher.Options
		agentFlagsStore types.KVStore
	}{
		{
			name: "happy path",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store, err := storageci.NewStore(t, log.NewNopLogger(), storage.AgentFlagsStore.String())
			require.NoError(t, err)
			fc := NewFlagController(log.NewNopLogger(), store)
			assert.NotNil(t, fc)

			var value bool
			assertValues := func(expectedValue bool) {
				value = fc.DesktopEnabled()
				assert.Equal(t, expectedValue, value)
				value = fc.DebugServerData()
				assert.Equal(t, expectedValue, value)
				value = fc.ForceControlSubsystems()
				assert.Equal(t, expectedValue, value)
				value = fc.DisableControlTLS()
				assert.Equal(t, expectedValue, value)
				value = fc.InsecureControlTLS()
			}

			assertValues(false)

			err = fc.SetDesktopEnabled(true)
			require.NoError(t, err)
			err = fc.SetDebugServerData(true)
			require.NoError(t, err)
			err = fc.SetForceControlSubsystems(true)
			require.NoError(t, err)
			err = fc.SetDisableControlTLS(true)
			require.NoError(t, err)
			err = fc.SetInsecureControlTLS(true)
			require.NoError(t, err)

			assertValues(true)
		})
	}
}

func TestControllerStringFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		cmdLineOpts     *launcher.Options
		agentFlagsStore types.KVStore
		valueToSet      string
	}{
		{
			name:       "happy path",
			valueToSet: "what.a.great.url.com",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store, err := storageci.NewStore(t, log.NewNopLogger(), storage.AgentFlagsStore.String())
			require.NoError(t, err)
			fc := NewFlagController(log.NewNopLogger(), store)
			assert.NotNil(t, fc)

			var value string
			assertValues := func(expectedValue string) {
				value = fc.ControlServerURL()
				assert.Equal(t, expectedValue, value)
			}

			assertValues("")

			err = fc.SetControlServerURL(tt.valueToSet)
			require.NoError(t, err)

			assertValues(tt.valueToSet)
		})
	}
}

func TestControllerDurationFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		cmdLineOpts     *launcher.Options
		agentFlagsStore types.KVStore
		valueToSet      time.Duration
	}{
		{
			name:       "happy path",
			valueToSet: 7 * time.Second,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store, err := storageci.NewStore(t, log.NewNopLogger(), storage.AgentFlagsStore.String())
			require.NoError(t, err)
			fc := NewFlagController(log.NewNopLogger(), store)
			assert.NotNil(t, fc)

			var value time.Duration
			assertValues := func(expectedValue time.Duration) {
				value = fc.ControlRequestInterval()
				assert.Equal(t, expectedValue, value)
			}

			assertValues(5 * time.Second)

			err = fc.SetControlRequestInterval(tt.valueToSet)
			require.NoError(t, err)

			assertValues(tt.valueToSet)
		})
	}
}

func TestControllerNotify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		valueToSet time.Duration
	}{
		{
			name:       "happy path",
			valueToSet: 8 * time.Second,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store, err := storageci.NewStore(t, log.NewNopLogger(), storage.AgentFlagsStore.String())
			require.NoError(t, err)
			fc := NewFlagController(log.NewNopLogger(), store)
			assert.NotNil(t, fc)

			mockObserver := mocks.NewFlagsChangeObserver(t)
			mockObserver.On("FlagsChanged", []keys.FlagKey{keys.ControlRequestInterval})

			fc.RegisterChangeObserver(mockObserver, keys.ControlRequestInterval)

			err = fc.SetControlRequestInterval(tt.valueToSet)
			require.NoError(t, err)

			value := fc.ControlRequestInterval()
			assert.Equal(t, tt.valueToSet, value)
		})
	}
}

func TestControllerUpdate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		kvPairs     map[string]string
		changedKeys []string
	}{
		{
			name:        "happy path",
			kvPairs:     map[string]string{keys.ControlRequestInterval.String(): "125000", keys.ControlServerURL.String(): "kolide-app.com"},
			changedKeys: []string{keys.ControlRequestInterval.String(), keys.ControlServerURL.String()},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store, err := storageci.NewStore(t, log.NewNopLogger(), storage.AgentFlagsStore.String())
			require.NoError(t, err)
			fc := NewFlagController(log.NewNopLogger(), store)
			assert.NotNil(t, fc)

			mockObserver := mocks.NewFlagsChangeObserver(t)
			mockObserver.On("FlagsChanged", mock.MatchedBy(func(k []keys.FlagKey) bool {
				return assert.ElementsMatch(t, k, keys.ToFlagKeys(tt.changedKeys))
			}))

			fc.RegisterChangeObserver(mockObserver, keys.ToFlagKeys(tt.changedKeys)...)

			changedKeys, err := fc.Update(tt.kvPairs)
			require.NoError(t, err)

			assert.ElementsMatch(t, tt.changedKeys, changedKeys)
		})
	}
}

func TestControllerOverride(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		valueToSet time.Duration
		interval   time.Duration
		duration   time.Duration
	}{
		{
			name:       "happy path",
			valueToSet: 8 * time.Second,
			interval:   6 * time.Second,
			duration:   2 * time.Second,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store, err := storageci.NewStore(t, log.NewNopLogger(), storage.AgentFlagsStore.String())
			require.NoError(t, err)
			fc := NewFlagController(log.NewNopLogger(), store)
			assert.NotNil(t, fc)

			mockObserver := mocks.NewFlagsChangeObserver(t)
			mockObserver.On("FlagsChanged", []keys.FlagKey{keys.ControlRequestInterval})

			fc.RegisterChangeObserver(mockObserver, keys.ControlRequestInterval)

			err = fc.SetControlRequestInterval(tt.valueToSet)
			require.NoError(t, err)

			value := fc.ControlRequestInterval()
			assert.Equal(t, tt.valueToSet, value)

			fc.SetControlRequestIntervalOverride(tt.interval, tt.duration)
			assert.Equal(t, tt.interval, fc.ControlRequestInterval())

			time.Sleep(tt.duration * 2)
			assert.Equal(t, tt.valueToSet, fc.ControlRequestInterval())
		})
	}
}
