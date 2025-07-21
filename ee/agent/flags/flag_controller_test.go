package flags

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"slices"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/log/multislogger"
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

			store, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.AgentFlagsStore.String())
			require.NoError(t, err)
			fc := NewFlagController(multislogger.NewNopLogger(), store)
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
				assert.Equal(t, expectedValue, value)
				value = fc.InsecureTLS()
				assert.Equal(t, expectedValue, value)
				value = fc.InsecureTransportTLS()
				assert.Equal(t, expectedValue, value)
				value = fc.Debug()
				assert.Equal(t, expectedValue, value)
				value = fc.OsqueryVerbose()
				assert.Equal(t, expectedValue, value)
				value = fc.Autoupdate()
				assert.Equal(t, expectedValue, value)
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
			err = fc.SetInsecureTLS(true)
			require.NoError(t, err)
			err = fc.SetInsecureTransportTLS(true)
			require.NoError(t, err)
			err = fc.SetDebug(true)
			require.NoError(t, err)
			err = fc.SetOsqueryVerbose(true)
			require.NoError(t, err)
			err = fc.SetAutoupdate(true)
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

			store, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.AgentFlagsStore.String())
			require.NoError(t, err)
			fc := NewFlagController(multislogger.NewNopLogger(), store)
			assert.NotNil(t, fc)

			var value string
			assertGettersValues := func(expectedValue string) {
				value = fc.EnrollSecret()
				assert.Equal(t, expectedValue, value)
				value = fc.EnrollSecretPath()
				assert.Equal(t, expectedValue, value)
				value = fc.RootDirectory()
				assert.Equal(t, expectedValue, value)
				value = fc.OsquerydPath()
				assert.Equal(t, expectedValue, value)
				value = fc.RootPEM()
				assert.Equal(t, expectedValue, value)
				value = fc.Transport()
				assert.Equal(t, expectedValue, value)
			}

			assertGettersValues("")

			assertValues := func(expectedValue string) {
				value = fc.KolideServerURL()
				assert.Equal(t, expectedValue, value)
				value = fc.ControlServerURL()
				assert.Equal(t, expectedValue, value)
				value = fc.TufServerURL()
				assert.Equal(t, expectedValue, value)
				value = fc.MirrorServerURL()
				assert.Equal(t, expectedValue, value)
				value = fc.UpdateDirectory()
				assert.Equal(t, expectedValue, value)
			}

			assertValues("")

			err = fc.SetKolideServerURL(tt.valueToSet)
			require.NoError(t, err)
			err = fc.SetControlServerURL(tt.valueToSet)
			require.NoError(t, err)
			err = fc.SetTufServerURL(tt.valueToSet)
			require.NoError(t, err)
			err = fc.SetMirrorServerURL(tt.valueToSet)
			require.NoError(t, err)
			err = fc.SetUpdateDirectory(tt.valueToSet)
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
		getFlag         func(fc *FlagController) time.Duration
		setFlag         func(fc *FlagController, d time.Duration) error
		valuesToSet     []time.Duration
		valuesToGet     []time.Duration
	}{
		{
			name:        "LoggingInterval",
			getFlag:     func(fc *FlagController) time.Duration { return fc.LoggingInterval() },
			setFlag:     func(fc *FlagController, d time.Duration) error { return fc.SetLoggingInterval(d) },
			valuesToSet: []time.Duration{1 * time.Second, 7 * time.Second, 20 * time.Minute},
			valuesToGet: []time.Duration{5 * time.Second, 7 * time.Second, 10 * time.Minute},
		},
		{
			name:        "DesktopUpdateInterval",
			getFlag:     func(fc *FlagController) time.Duration { return fc.DesktopUpdateInterval() },
			setFlag:     func(fc *FlagController, d time.Duration) error { return fc.SetDesktopUpdateInterval(d) },
			valuesToSet: []time.Duration{1 * time.Second, 7 * time.Second, 20 * time.Minute},
			valuesToGet: []time.Duration{5 * time.Second, 7 * time.Second, 10 * time.Minute},
		},
		{
			name:        "DesktopMenuRefreshInterval",
			getFlag:     func(fc *FlagController) time.Duration { return fc.DesktopMenuRefreshInterval() },
			setFlag:     func(fc *FlagController, d time.Duration) error { return fc.SetDesktopMenuRefreshInterval(d) },
			valuesToSet: []time.Duration{1 * time.Second, 7 * time.Minute, 30 * time.Minute},
			valuesToGet: []time.Duration{5 * time.Minute, 7 * time.Minute, 30 * time.Minute},
		},
		{
			name:        "ControlRequestInterval",
			getFlag:     func(fc *FlagController) time.Duration { return fc.ControlRequestInterval() },
			setFlag:     func(fc *FlagController, d time.Duration) error { return fc.SetControlRequestInterval(d) },
			valuesToSet: []time.Duration{1 * time.Second, 7 * time.Second, 20 * time.Minute},
			valuesToGet: []time.Duration{5 * time.Second, 7 * time.Second, 10 * time.Minute},
		},
		{
			name:        "AutoupdateInterval",
			getFlag:     func(fc *FlagController) time.Duration { return fc.AutoupdateInterval() },
			setFlag:     func(fc *FlagController, d time.Duration) error { return fc.SetAutoupdateInterval(d) },
			valuesToSet: []time.Duration{1 * time.Second, 30 * time.Minute, 36 * time.Hour},
			valuesToGet: []time.Duration{1 * time.Minute, 30 * time.Minute, 24 * time.Hour},
		},
		{
			name:        "AutoupdateInitialDelay",
			getFlag:     func(fc *FlagController) time.Duration { return fc.AutoupdateInitialDelay() },
			setFlag:     func(fc *FlagController, d time.Duration) error { return fc.SetAutoupdateInitialDelay(d) },
			valuesToSet: []time.Duration{1 * time.Second, 7 * time.Second, 20 * time.Minute},
			valuesToGet: []time.Duration{5 * time.Second, 7 * time.Second, 20 * time.Minute},
		},
		{
			name:        "AutoupdateDownloadSplay",
			getFlag:     func(fc *FlagController) time.Duration { return fc.AutoupdateDownloadSplay() },
			setFlag:     func(fc *FlagController, d time.Duration) error { return fc.SetAutoupdateDownloadSplay(d) },
			valuesToSet: []time.Duration{0 * time.Second, 12 * time.Hour, 110 * time.Hour},
			valuesToGet: []time.Duration{0 * time.Second, 12 * time.Hour, 72 * time.Hour},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.AgentFlagsStore.String())
			require.NoError(t, err)
			fc := NewFlagController(multislogger.NewNopLogger(), store)
			assert.NotNil(t, fc)

			for i, valueToSet := range tt.valuesToSet {
				err = tt.setFlag(fc, valueToSet)
				require.NoError(t, err)

				value := tt.getFlag(fc)
				assert.Equal(t, tt.valuesToGet[i], value)
			}
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

			store, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.AgentFlagsStore.String())
			require.NoError(t, err)
			fc := NewFlagController(multislogger.NewNopLogger(), store)
			assert.NotNil(t, fc)

			mockObserver := mocks.NewFlagsChangeObserver(t)
			mockObserver.On("FlagsChanged", mock.Anything, keys.ControlRequestInterval)

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

			store, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.AgentFlagsStore.String())
			require.NoError(t, err)
			fc := NewFlagController(multislogger.NewNopLogger(), store)
			assert.NotNil(t, fc)

			mockObserver := mocks.NewFlagsChangeObserver(t)
			matchKey := mock.MatchedBy(func(k keys.FlagKey) bool {
				return assert.Contains(t, keys.ToFlagKeys(tt.changedKeys), k)
			})
			mockObserver.On("FlagsChanged", mock.Anything, matchKey, matchKey)

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
			valueToSet: 8 * time.Second, // cannot be below 5 seconds for control request interval
			interval:   6 * time.Second, // cannot be below 5 seconds for control request interval
			duration:   100 * time.Millisecond,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.AgentFlagsStore.String())
			require.NoError(t, err)
			fc := NewFlagController(multislogger.NewNopLogger(), store)
			assert.NotNil(t, fc)

			mockObserver := mocks.NewFlagsChangeObserver(t)
			mockObserver.On("FlagsChanged", mock.Anything, keys.ControlRequestInterval)

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

func TestDeregisterChangeObserver(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		valueToSet time.Duration
		interval   time.Duration
		duration   time.Duration
	}{
		{
			name:       "happy path",
			valueToSet: 8 * time.Second, // cannot be below 5 seconds for control request interval
			interval:   6 * time.Second, // cannot be below 5 seconds for control request interval
			duration:   100 * time.Millisecond,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.AgentFlagsStore.String())
			require.NoError(t, err)
			fc := NewFlagController(multislogger.NewNopLogger(), store)
			assert.NotNil(t, fc)

			// Create a change observer and register it
			mockObserver := mocks.NewFlagsChangeObserver(t)
			mockObserver.On("FlagsChanged", mock.Anything, keys.ControlRequestInterval).Once()
			fc.RegisterChangeObserver(mockObserver, keys.ControlRequestInterval)

			// Set the control request interval -- the mock observer will have `FlagsChanged` called
			err = fc.SetControlRequestInterval(tt.valueToSet)
			require.NoError(t, err)

			value := fc.ControlRequestInterval()
			assert.Equal(t, tt.valueToSet, value)

			// Now, deregister the change observer
			fc.DeregisterChangeObserver(mockObserver)
			require.NotContains(t, fc.observers, mockObserver)

			// Set the control request interval again -- the mock observer will NOT have `FlagsChanged` called
			fc.SetControlRequestIntervalOverride(tt.interval, tt.duration)
			assert.Equal(t, tt.interval, fc.ControlRequestInterval())

			mockObserver.AssertExpectations(t)
		})
	}
}

type deadlockedObserver struct {
	flags                types.Flags
	observerKey          keys.FlagKey
	flagsChangedCalled   *atomic.Bool
	flagsChangedReturned *atomic.Bool
}

func newDeadlockedObserver(f types.Flags, observerKey keys.FlagKey) *deadlockedObserver {
	return &deadlockedObserver{
		flags:                f,
		observerKey:          observerKey,
		flagsChangedCalled:   &atomic.Bool{},
		flagsChangedReturned: &atomic.Bool{},
	}
}

func (d *deadlockedObserver) FlagsChanged(ctx context.Context, flagKeys ...keys.FlagKey) {
	d.flagsChangedCalled.Store(true)
	// This simulates the control service acquiring the Fetch lock, performing Fetch, getting katc_config updates,
	// creating a new table with the updated config, and then trying to register that table as a change observer
	// with the flag controller (so that the table can respond to changes in `d.observerKey` i.e. TableGenerateTimeout).
	d.flags.RegisterChangeObserver(d, d.observerKey)
	d.flagsChangedReturned.Store(true)
}

func TestObserverDeadlock(t *testing.T) {
	t.Parallel()

	store, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.AgentFlagsStore.String())
	require.NoError(t, err)
	fc := NewFlagController(multislogger.NewNopLogger(), store)
	assert.NotNil(t, fc)

	// Set up an observer that will observe changes to one key (ControlRequestInterval)
	// and on change to that interval, will set up a new observer for a different key.
	newObserverKey := keys.TableGenerateTimeout
	d := newDeadlockedObserver(fc, newObserverKey)
	fc.RegisterChangeObserver(d, keys.ControlRequestInterval)

	// Now, set up an override
	overrideDuration := 30 * time.Second
	setOverrideReturned := make(chan struct{})

	go func() {
		fc.SetControlRequestIntervalOverride(3*time.Second, overrideDuration)
		setOverrideReturned <- struct{}{}
	}()

	select {
	case <-setOverrideReturned:
	case <-time.After(30 * time.Second):
		t.Error("could not set control request override within 30 seconds")
		t.FailNow()
	}

	// Wait for the override to expire
	time.Sleep(2 * overrideDuration)

	// See whether override expired
	fc.overrideMutex.RLock()
	require.Equal(t, 0, len(fc.overrides), "override not removed")
	fc.overrideMutex.RUnlock()

	// Make sure that FlagsChanged was not held open by the observersMutex
	require.True(t, d.flagsChangedCalled.Load(), "FlagsChanged not called")
	require.True(t, d.flagsChangedReturned.Load(), "FlagsChanged did not return")

	// Make sure that there's an observer registered for d.observerKey
	fc.observersMutex.RLock()
	observerForKeyFound := false
	for _, keys := range fc.observers {
		if slices.Contains(keys, newObserverKey) {
			observerForKeyFound = true
		}
	}
	fc.observersMutex.RUnlock()
	require.True(t, observerForKeyFound, "new observer not successfully registered")
}
