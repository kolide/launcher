// Code generated by mockery v2.45.0. DO NOT EDIT.

package mocks

import (
	keys "github.com/kolide/launcher/ee/agent/flags/keys"
	mock "github.com/stretchr/testify/mock"

	time "time"

	types "github.com/kolide/launcher/ee/agent/types"
)

// Flags is an autogenerated mock type for the Flags type
type Flags struct {
	mock.Mock
}

// Autoupdate provides a mock function with given fields:
func (_m *Flags) Autoupdate() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Autoupdate")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// AutoupdateInitialDelay provides a mock function with given fields:
func (_m *Flags) AutoupdateInitialDelay() time.Duration {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for AutoupdateInitialDelay")
	}

	var r0 time.Duration
	if rf, ok := ret.Get(0).(func() time.Duration); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(time.Duration)
	}

	return r0
}

// AutoupdateInterval provides a mock function with given fields:
func (_m *Flags) AutoupdateInterval() time.Duration {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for AutoupdateInterval")
	}

	var r0 time.Duration
	if rf, ok := ret.Get(0).(func() time.Duration); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(time.Duration)
	}

	return r0
}

// CertPins provides a mock function with given fields:
func (_m *Flags) CertPins() [][]byte {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for CertPins")
	}

	var r0 [][]byte
	if rf, ok := ret.Get(0).(func() [][]byte); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([][]byte)
		}
	}

	return r0
}

// ControlRequestInterval provides a mock function with given fields:
func (_m *Flags) ControlRequestInterval() time.Duration {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for ControlRequestInterval")
	}

	var r0 time.Duration
	if rf, ok := ret.Get(0).(func() time.Duration); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(time.Duration)
	}

	return r0
}

// ControlServerURL provides a mock function with given fields:
func (_m *Flags) ControlServerURL() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for ControlServerURL")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// Debug provides a mock function with given fields:
func (_m *Flags) Debug() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Debug")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// DebugLogFile provides a mock function with given fields:
func (_m *Flags) DebugLogFile() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for DebugLogFile")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// DebugServerData provides a mock function with given fields:
func (_m *Flags) DebugServerData() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for DebugServerData")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// DesktopEnabled provides a mock function with given fields:
func (_m *Flags) DesktopEnabled() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for DesktopEnabled")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// DesktopMenuRefreshInterval provides a mock function with given fields:
func (_m *Flags) DesktopMenuRefreshInterval() time.Duration {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for DesktopMenuRefreshInterval")
	}

	var r0 time.Duration
	if rf, ok := ret.Get(0).(func() time.Duration); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(time.Duration)
	}

	return r0
}

// DesktopUpdateInterval provides a mock function with given fields:
func (_m *Flags) DesktopUpdateInterval() time.Duration {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for DesktopUpdateInterval")
	}

	var r0 time.Duration
	if rf, ok := ret.Get(0).(func() time.Duration); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(time.Duration)
	}

	return r0
}

// DisableControlTLS provides a mock function with given fields:
func (_m *Flags) DisableControlTLS() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for DisableControlTLS")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// DisableTraceIngestTLS provides a mock function with given fields:
func (_m *Flags) DisableTraceIngestTLS() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for DisableTraceIngestTLS")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// EnableInitialRunner provides a mock function with given fields:
func (_m *Flags) EnableInitialRunner() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for EnableInitialRunner")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// EnrollSecret provides a mock function with given fields:
func (_m *Flags) EnrollSecret() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for EnrollSecret")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// EnrollSecretPath provides a mock function with given fields:
func (_m *Flags) EnrollSecretPath() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for EnrollSecretPath")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// ExportTraces provides a mock function with given fields:
func (_m *Flags) ExportTraces() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for ExportTraces")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// ForceControlSubsystems provides a mock function with given fields:
func (_m *Flags) ForceControlSubsystems() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for ForceControlSubsystems")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// IAmBreakingEELicense provides a mock function with given fields:
func (_m *Flags) IAmBreakingEELicense() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for IAmBreakingEELicense")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// InModernStandby provides a mock function with given fields:
func (_m *Flags) InModernStandby() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for InModernStandby")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// InsecureControlTLS provides a mock function with given fields:
func (_m *Flags) InsecureControlTLS() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for InsecureControlTLS")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// InsecureTLS provides a mock function with given fields:
func (_m *Flags) InsecureTLS() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for InsecureTLS")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// InsecureTransportTLS provides a mock function with given fields:
func (_m *Flags) InsecureTransportTLS() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for InsecureTransportTLS")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// KolideHosted provides a mock function with given fields:
func (_m *Flags) KolideHosted() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for KolideHosted")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// KolideServerURL provides a mock function with given fields:
func (_m *Flags) KolideServerURL() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for KolideServerURL")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// LauncherWatchdogEnabled provides a mock function with given fields:
func (_m *Flags) LauncherWatchdogEnabled() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for LauncherWatchdogEnabled")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// LocalDevelopmentPath provides a mock function with given fields:
func (_m *Flags) LocalDevelopmentPath() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for LocalDevelopmentPath")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// LogIngestServerURL provides a mock function with given fields:
func (_m *Flags) LogIngestServerURL() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for LogIngestServerURL")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// LogMaxBytesPerBatch provides a mock function with given fields:
func (_m *Flags) LogMaxBytesPerBatch() int {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for LogMaxBytesPerBatch")
	}

	var r0 int
	if rf, ok := ret.Get(0).(func() int); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(int)
	}

	return r0
}

// LogShippingLevel provides a mock function with given fields:
func (_m *Flags) LogShippingLevel() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for LogShippingLevel")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// LoggingInterval provides a mock function with given fields:
func (_m *Flags) LoggingInterval() time.Duration {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for LoggingInterval")
	}

	var r0 time.Duration
	if rf, ok := ret.Get(0).(func() time.Duration); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(time.Duration)
	}

	return r0
}

// MirrorServerURL provides a mock function with given fields:
func (_m *Flags) MirrorServerURL() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for MirrorServerURL")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// OsqueryFlags provides a mock function with given fields:
func (_m *Flags) OsqueryFlags() []string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for OsqueryFlags")
	}

	var r0 []string
	if rf, ok := ret.Get(0).(func() []string); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]string)
		}
	}

	return r0
}

// OsqueryHealthcheckStartupDelay provides a mock function with given fields:
func (_m *Flags) OsqueryHealthcheckStartupDelay() time.Duration {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for OsqueryHealthcheckStartupDelay")
	}

	var r0 time.Duration
	if rf, ok := ret.Get(0).(func() time.Duration); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(time.Duration)
	}

	return r0
}

// OsqueryTlsConfigEndpoint provides a mock function with given fields:
func (_m *Flags) OsqueryTlsConfigEndpoint() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for OsqueryTlsConfigEndpoint")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// OsqueryTlsDistributedReadEndpoint provides a mock function with given fields:
func (_m *Flags) OsqueryTlsDistributedReadEndpoint() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for OsqueryTlsDistributedReadEndpoint")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// OsqueryTlsDistributedWriteEndpoint provides a mock function with given fields:
func (_m *Flags) OsqueryTlsDistributedWriteEndpoint() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for OsqueryTlsDistributedWriteEndpoint")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// OsqueryTlsEnrollEndpoint provides a mock function with given fields:
func (_m *Flags) OsqueryTlsEnrollEndpoint() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for OsqueryTlsEnrollEndpoint")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// OsqueryTlsLoggerEndpoint provides a mock function with given fields:
func (_m *Flags) OsqueryTlsLoggerEndpoint() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for OsqueryTlsLoggerEndpoint")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// OsqueryVerbose provides a mock function with given fields:
func (_m *Flags) OsqueryVerbose() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for OsqueryVerbose")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// OsquerydPath provides a mock function with given fields:
func (_m *Flags) OsquerydPath() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for OsquerydPath")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// PinnedLauncherVersion provides a mock function with given fields:
func (_m *Flags) PinnedLauncherVersion() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for PinnedLauncherVersion")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// PinnedOsquerydVersion provides a mock function with given fields:
func (_m *Flags) PinnedOsquerydVersion() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for PinnedOsquerydVersion")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// RegisterChangeObserver provides a mock function with given fields: observer, flagKeys
func (_m *Flags) RegisterChangeObserver(observer types.FlagsChangeObserver, flagKeys ...keys.FlagKey) {
	_va := make([]interface{}, len(flagKeys))
	for _i := range flagKeys {
		_va[_i] = flagKeys[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, observer)
	_ca = append(_ca, _va...)
	_m.Called(_ca...)
}

// RootDirectory provides a mock function with given fields:
func (_m *Flags) RootDirectory() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for RootDirectory")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// RootPEM provides a mock function with given fields:
func (_m *Flags) RootPEM() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for RootPEM")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// SetAutoupdate provides a mock function with given fields: enabled
func (_m *Flags) SetAutoupdate(enabled bool) error {
	ret := _m.Called(enabled)

	if len(ret) == 0 {
		panic("no return value specified for SetAutoupdate")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(bool) error); ok {
		r0 = rf(enabled)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetAutoupdateInitialDelay provides a mock function with given fields: delay
func (_m *Flags) SetAutoupdateInitialDelay(delay time.Duration) error {
	ret := _m.Called(delay)

	if len(ret) == 0 {
		panic("no return value specified for SetAutoupdateInitialDelay")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(time.Duration) error); ok {
		r0 = rf(delay)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetAutoupdateInterval provides a mock function with given fields: interval
func (_m *Flags) SetAutoupdateInterval(interval time.Duration) error {
	ret := _m.Called(interval)

	if len(ret) == 0 {
		panic("no return value specified for SetAutoupdateInterval")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(time.Duration) error); ok {
		r0 = rf(interval)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetControlRequestInterval provides a mock function with given fields: interval
func (_m *Flags) SetControlRequestInterval(interval time.Duration) error {
	ret := _m.Called(interval)

	if len(ret) == 0 {
		panic("no return value specified for SetControlRequestInterval")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(time.Duration) error); ok {
		r0 = rf(interval)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetControlRequestIntervalOverride provides a mock function with given fields: value, duration
func (_m *Flags) SetControlRequestIntervalOverride(value time.Duration, duration time.Duration) {
	_m.Called(value, duration)
}

// SetControlServerURL provides a mock function with given fields: url
func (_m *Flags) SetControlServerURL(url string) error {
	ret := _m.Called(url)

	if len(ret) == 0 {
		panic("no return value specified for SetControlServerURL")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(url)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetDebug provides a mock function with given fields: debug
func (_m *Flags) SetDebug(debug bool) error {
	ret := _m.Called(debug)

	if len(ret) == 0 {
		panic("no return value specified for SetDebug")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(bool) error); ok {
		r0 = rf(debug)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetDebugServerData provides a mock function with given fields: debug
func (_m *Flags) SetDebugServerData(debug bool) error {
	ret := _m.Called(debug)

	if len(ret) == 0 {
		panic("no return value specified for SetDebugServerData")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(bool) error); ok {
		r0 = rf(debug)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetDesktopEnabled provides a mock function with given fields: enabled
func (_m *Flags) SetDesktopEnabled(enabled bool) error {
	ret := _m.Called(enabled)

	if len(ret) == 0 {
		panic("no return value specified for SetDesktopEnabled")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(bool) error); ok {
		r0 = rf(enabled)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetDesktopMenuRefreshInterval provides a mock function with given fields: interval
func (_m *Flags) SetDesktopMenuRefreshInterval(interval time.Duration) error {
	ret := _m.Called(interval)

	if len(ret) == 0 {
		panic("no return value specified for SetDesktopMenuRefreshInterval")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(time.Duration) error); ok {
		r0 = rf(interval)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetDesktopUpdateInterval provides a mock function with given fields: interval
func (_m *Flags) SetDesktopUpdateInterval(interval time.Duration) error {
	ret := _m.Called(interval)

	if len(ret) == 0 {
		panic("no return value specified for SetDesktopUpdateInterval")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(time.Duration) error); ok {
		r0 = rf(interval)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetDisableControlTLS provides a mock function with given fields: disabled
func (_m *Flags) SetDisableControlTLS(disabled bool) error {
	ret := _m.Called(disabled)

	if len(ret) == 0 {
		panic("no return value specified for SetDisableControlTLS")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(bool) error); ok {
		r0 = rf(disabled)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetDisableTraceIngestTLS provides a mock function with given fields: enabled
func (_m *Flags) SetDisableTraceIngestTLS(enabled bool) error {
	ret := _m.Called(enabled)

	if len(ret) == 0 {
		panic("no return value specified for SetDisableTraceIngestTLS")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(bool) error); ok {
		r0 = rf(enabled)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetExportTraces provides a mock function with given fields: enabled
func (_m *Flags) SetExportTraces(enabled bool) error {
	ret := _m.Called(enabled)

	if len(ret) == 0 {
		panic("no return value specified for SetExportTraces")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(bool) error); ok {
		r0 = rf(enabled)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetExportTracesOverride provides a mock function with given fields: value, duration
func (_m *Flags) SetExportTracesOverride(value bool, duration time.Duration) {
	_m.Called(value, duration)
}

// SetForceControlSubsystems provides a mock function with given fields: force
func (_m *Flags) SetForceControlSubsystems(force bool) error {
	ret := _m.Called(force)

	if len(ret) == 0 {
		panic("no return value specified for SetForceControlSubsystems")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(bool) error); ok {
		r0 = rf(force)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetInModernStandby provides a mock function with given fields: enabled
func (_m *Flags) SetInModernStandby(enabled bool) error {
	ret := _m.Called(enabled)

	if len(ret) == 0 {
		panic("no return value specified for SetInModernStandby")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(bool) error); ok {
		r0 = rf(enabled)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetInsecureControlTLS provides a mock function with given fields: disabled
func (_m *Flags) SetInsecureControlTLS(disabled bool) error {
	ret := _m.Called(disabled)

	if len(ret) == 0 {
		panic("no return value specified for SetInsecureControlTLS")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(bool) error); ok {
		r0 = rf(disabled)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetInsecureTLS provides a mock function with given fields: insecure
func (_m *Flags) SetInsecureTLS(insecure bool) error {
	ret := _m.Called(insecure)

	if len(ret) == 0 {
		panic("no return value specified for SetInsecureTLS")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(bool) error); ok {
		r0 = rf(insecure)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetInsecureTransportTLS provides a mock function with given fields: insecure
func (_m *Flags) SetInsecureTransportTLS(insecure bool) error {
	ret := _m.Called(insecure)

	if len(ret) == 0 {
		panic("no return value specified for SetInsecureTransportTLS")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(bool) error); ok {
		r0 = rf(insecure)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetKolideServerURL provides a mock function with given fields: url
func (_m *Flags) SetKolideServerURL(url string) error {
	ret := _m.Called(url)

	if len(ret) == 0 {
		panic("no return value specified for SetKolideServerURL")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(url)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetLauncherWatchdogEnabled provides a mock function with given fields: enabled
func (_m *Flags) SetLauncherWatchdogEnabled(enabled bool) error {
	ret := _m.Called(enabled)

	if len(ret) == 0 {
		panic("no return value specified for SetLauncherWatchdogEnabled")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(bool) error); ok {
		r0 = rf(enabled)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetLogIngestServerURL provides a mock function with given fields: url
func (_m *Flags) SetLogIngestServerURL(url string) error {
	ret := _m.Called(url)

	if len(ret) == 0 {
		panic("no return value specified for SetLogIngestServerURL")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(url)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetLogShippingLevel provides a mock function with given fields: level
func (_m *Flags) SetLogShippingLevel(level string) error {
	ret := _m.Called(level)

	if len(ret) == 0 {
		panic("no return value specified for SetLogShippingLevel")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(level)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetLogShippingLevelOverride provides a mock function with given fields: value, duration
func (_m *Flags) SetLogShippingLevelOverride(value string, duration time.Duration) {
	_m.Called(value, duration)
}

// SetLoggingInterval provides a mock function with given fields: interval
func (_m *Flags) SetLoggingInterval(interval time.Duration) error {
	ret := _m.Called(interval)

	if len(ret) == 0 {
		panic("no return value specified for SetLoggingInterval")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(time.Duration) error); ok {
		r0 = rf(interval)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetMirrorServerURL provides a mock function with given fields: url
func (_m *Flags) SetMirrorServerURL(url string) error {
	ret := _m.Called(url)

	if len(ret) == 0 {
		panic("no return value specified for SetMirrorServerURL")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(url)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetOsqueryHealthcheckStartupDelay provides a mock function with given fields: delay
func (_m *Flags) SetOsqueryHealthcheckStartupDelay(delay time.Duration) error {
	ret := _m.Called(delay)

	if len(ret) == 0 {
		panic("no return value specified for SetOsqueryHealthcheckStartupDelay")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(time.Duration) error); ok {
		r0 = rf(delay)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetOsqueryVerbose provides a mock function with given fields: verbose
func (_m *Flags) SetOsqueryVerbose(verbose bool) error {
	ret := _m.Called(verbose)

	if len(ret) == 0 {
		panic("no return value specified for SetOsqueryVerbose")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(bool) error); ok {
		r0 = rf(verbose)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetPinnedLauncherVersion provides a mock function with given fields: version
func (_m *Flags) SetPinnedLauncherVersion(version string) error {
	ret := _m.Called(version)

	if len(ret) == 0 {
		panic("no return value specified for SetPinnedLauncherVersion")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(version)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetPinnedOsquerydVersion provides a mock function with given fields: version
func (_m *Flags) SetPinnedOsquerydVersion(version string) error {
	ret := _m.Called(version)

	if len(ret) == 0 {
		panic("no return value specified for SetPinnedOsquerydVersion")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(version)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetSystrayRestartEnabled provides a mock function with given fields: enabled
func (_m *Flags) SetSystrayRestartEnabled(enabled bool) error {
	ret := _m.Called(enabled)

	if len(ret) == 0 {
		panic("no return value specified for SetSystrayRestartEnabled")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(bool) error); ok {
		r0 = rf(enabled)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetTraceBatchTimeout provides a mock function with given fields: duration
func (_m *Flags) SetTraceBatchTimeout(duration time.Duration) error {
	ret := _m.Called(duration)

	if len(ret) == 0 {
		panic("no return value specified for SetTraceBatchTimeout")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(time.Duration) error); ok {
		r0 = rf(duration)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetTraceIngestServerURL provides a mock function with given fields: url
func (_m *Flags) SetTraceIngestServerURL(url string) error {
	ret := _m.Called(url)

	if len(ret) == 0 {
		panic("no return value specified for SetTraceIngestServerURL")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(url)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetTraceSamplingRate provides a mock function with given fields: rate
func (_m *Flags) SetTraceSamplingRate(rate float64) error {
	ret := _m.Called(rate)

	if len(ret) == 0 {
		panic("no return value specified for SetTraceSamplingRate")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(float64) error); ok {
		r0 = rf(rate)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetTraceSamplingRateOverride provides a mock function with given fields: value, duration
func (_m *Flags) SetTraceSamplingRateOverride(value float64, duration time.Duration) {
	_m.Called(value, duration)
}

// SetTufServerURL provides a mock function with given fields: url
func (_m *Flags) SetTufServerURL(url string) error {
	ret := _m.Called(url)

	if len(ret) == 0 {
		panic("no return value specified for SetTufServerURL")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(url)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetUpdateChannel provides a mock function with given fields: channel
func (_m *Flags) SetUpdateChannel(channel string) error {
	ret := _m.Called(channel)

	if len(ret) == 0 {
		panic("no return value specified for SetUpdateChannel")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(channel)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetUpdateDirectory provides a mock function with given fields: directory
func (_m *Flags) SetUpdateDirectory(directory string) error {
	ret := _m.Called(directory)

	if len(ret) == 0 {
		panic("no return value specified for SetUpdateDirectory")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(directory)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetWatchdogDelaySec provides a mock function with given fields: sec
func (_m *Flags) SetWatchdogDelaySec(sec int) error {
	ret := _m.Called(sec)

	if len(ret) == 0 {
		panic("no return value specified for SetWatchdogDelaySec")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(int) error); ok {
		r0 = rf(sec)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetWatchdogEnabled provides a mock function with given fields: enable
func (_m *Flags) SetWatchdogEnabled(enable bool) error {
	ret := _m.Called(enable)

	if len(ret) == 0 {
		panic("no return value specified for SetWatchdogEnabled")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(bool) error); ok {
		r0 = rf(enable)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetWatchdogMemoryLimitMB provides a mock function with given fields: limit
func (_m *Flags) SetWatchdogMemoryLimitMB(limit int) error {
	ret := _m.Called(limit)

	if len(ret) == 0 {
		panic("no return value specified for SetWatchdogMemoryLimitMB")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(int) error); ok {
		r0 = rf(limit)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetWatchdogUtilizationLimitPercent provides a mock function with given fields: limit
func (_m *Flags) SetWatchdogUtilizationLimitPercent(limit int) error {
	ret := _m.Called(limit)

	if len(ret) == 0 {
		panic("no return value specified for SetWatchdogUtilizationLimitPercent")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(int) error); ok {
		r0 = rf(limit)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SystrayRestartEnabled provides a mock function with given fields:
func (_m *Flags) SystrayRestartEnabled() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for SystrayRestartEnabled")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// TraceBatchTimeout provides a mock function with given fields:
func (_m *Flags) TraceBatchTimeout() time.Duration {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for TraceBatchTimeout")
	}

	var r0 time.Duration
	if rf, ok := ret.Get(0).(func() time.Duration); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(time.Duration)
	}

	return r0
}

// TraceIngestServerURL provides a mock function with given fields:
func (_m *Flags) TraceIngestServerURL() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for TraceIngestServerURL")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// TraceSamplingRate provides a mock function with given fields:
func (_m *Flags) TraceSamplingRate() float64 {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for TraceSamplingRate")
	}

	var r0 float64
	if rf, ok := ret.Get(0).(func() float64); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(float64)
	}

	return r0
}

// Transport provides a mock function with given fields:
func (_m *Flags) Transport() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Transport")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// TufServerURL provides a mock function with given fields:
func (_m *Flags) TufServerURL() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for TufServerURL")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// UpdateChannel provides a mock function with given fields:
func (_m *Flags) UpdateChannel() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for UpdateChannel")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// UpdateDirectory provides a mock function with given fields:
func (_m *Flags) UpdateDirectory() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for UpdateDirectory")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// WatchdogDelaySec provides a mock function with given fields:
func (_m *Flags) WatchdogDelaySec() int {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for WatchdogDelaySec")
	}

	var r0 int
	if rf, ok := ret.Get(0).(func() int); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(int)
	}

	return r0
}

// WatchdogEnabled provides a mock function with given fields:
func (_m *Flags) WatchdogEnabled() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for WatchdogEnabled")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// WatchdogMemoryLimitMB provides a mock function with given fields:
func (_m *Flags) WatchdogMemoryLimitMB() int {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for WatchdogMemoryLimitMB")
	}

	var r0 int
	if rf, ok := ret.Get(0).(func() int); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(int)
	}

	return r0
}

// WatchdogUtilizationLimitPercent provides a mock function with given fields:
func (_m *Flags) WatchdogUtilizationLimitPercent() int {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for WatchdogUtilizationLimitPercent")
	}

	var r0 int
	if rf, ok := ret.Get(0).(func() int); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(int)
	}

	return r0
}

// NewFlags creates a new instance of Flags. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewFlags(t interface {
	mock.TestingT
	Cleanup(func())
}) *Flags {
	mock := &Flags{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
