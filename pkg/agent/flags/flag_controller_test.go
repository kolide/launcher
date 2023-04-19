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
				value = fc.KolideHosted()
				assert.Equal(t, expectedValue, value)
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
				value = fc.IAmBreakingEELicense()
				assert.Equal(t, expectedValue, value)
				value = fc.Debug()
				assert.Equal(t, expectedValue, value)
				value = fc.OsqueryVerbose()
				assert.Equal(t, expectedValue, value)
				value = fc.Autoupdate()
				assert.Equal(t, expectedValue, value)
			}

			assertValues(false)

			err = fc.SetKolideHosted(true)
			require.NoError(t, err)
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
			err = fc.SetIAmBreakingEELicense(true)
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

			store, err := storageci.NewStore(t, log.NewNopLogger(), storage.AgentFlagsStore.String())
			require.NoError(t, err)
			fc := NewFlagController(log.NewNopLogger(), store)
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
				value = fc.OsqueryTlsConfigEndpoint()
				assert.Equal(t, expectedValue, value)
				value = fc.OsqueryTlsEnrollEndpoint()
				assert.Equal(t, expectedValue, value)
				value = fc.OsqueryTlsLoggerEndpoint()
				assert.Equal(t, expectedValue, value)
				value = fc.OsqueryTlsDistributedReadEndpoint()
				assert.Equal(t, expectedValue, value)
				value = fc.OsqueryTlsDistributedWriteEndpoint()
				assert.Equal(t, expectedValue, value)
			}

			assertGettersValues("")

			assertValues := func(expectedValue string) {
				value = fc.KolideServerURL()
				assert.Equal(t, expectedValue, value)
				value = fc.ControlServerURL()
				assert.Equal(t, expectedValue, value)
				value = fc.DebugLogFile()
				assert.Equal(t, expectedValue, value)
				value = fc.NotaryServerURL()
				assert.Equal(t, expectedValue, value)
				value = fc.TufServerURL()
				assert.Equal(t, expectedValue, value)
				value = fc.MirrorServerURL()
				assert.Equal(t, expectedValue, value)
				value = fc.NotaryPrefix()
				assert.Equal(t, expectedValue, value)
				value = fc.UpdateDirectory()
				assert.Equal(t, expectedValue, value)
			}

			assertValues("")

			err = fc.SetKolideServerURL(tt.valueToSet)
			require.NoError(t, err)
			err = fc.SetControlServerURL(tt.valueToSet)
			require.NoError(t, err)
			err = fc.SetDebugLogFile(tt.valueToSet)
			require.NoError(t, err)
			err = fc.SetNotaryServerURL(tt.valueToSet)
			require.NoError(t, err)
			err = fc.SetTufServerURL(tt.valueToSet)
			require.NoError(t, err)
			err = fc.SetMirrorServerURL(tt.valueToSet)
			require.NoError(t, err)
			err = fc.SetNotaryPrefix(tt.valueToSet)
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
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store, err := storageci.NewStore(t, log.NewNopLogger(), storage.AgentFlagsStore.String())
			require.NoError(t, err)
			fc := NewFlagController(log.NewNopLogger(), store)
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
