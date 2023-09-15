// Code generated by mockery v2.30.1. DO NOT EDIT.

package mocks

import (
	keys "github.com/kolide/launcher/pkg/agent/flags/keys"
	mock "github.com/stretchr/testify/mock"

	time "time"

	types "github.com/kolide/launcher/pkg/agent/types"
)

// Flags is an autogenerated mock type for the Flags type
type Flags struct {
	mock.Mock
}

// AutoloadedExtensions provides a mock function with given fields:
func (_m *Flags) AutoloadedExtensions() []string {
	ret := _m.Called()

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

// Autoupdate provides a mock function with given fields:
func (_m *Flags) Autoupdate() bool {
	ret := _m.Called()

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

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// DebugUploadRequestURL provides a mock function with given fields:
func (_m *Flags) DebugUploadRequestURL() string {
	ret := _m.Called()

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// DesktopEnabled provides a mock function with given fields:
func (_m *Flags) DesktopEnabled() bool {
	ret := _m.Called()

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

	var r0 int
	if rf, ok := ret.Get(0).(func() int); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(int)
	}

	return r0
}

// LoggingInterval provides a mock function with given fields:
func (_m *Flags) LoggingInterval() time.Duration {
	ret := _m.Called()

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

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// NotaryPrefix provides a mock function with given fields:
func (_m *Flags) NotaryPrefix() string {
	ret := _m.Called()

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// NotaryServerURL provides a mock function with given fields:
func (_m *Flags) NotaryServerURL() string {
	ret := _m.Called()

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

	var r0 error
	if rf, ok := ret.Get(0).(func(time.Duration) error); ok {
		r0 = rf(interval)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetControlRequestIntervalOverride provides a mock function with given fields: interval, duration
func (_m *Flags) SetControlRequestIntervalOverride(interval time.Duration, duration time.Duration) {
	_m.Called(interval, duration)
}

// SetControlServerURL provides a mock function with given fields: url
func (_m *Flags) SetControlServerURL(url string) error {
	ret := _m.Called(url)

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

	var r0 error
	if rf, ok := ret.Get(0).(func(bool) error); ok {
		r0 = rf(debug)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetDebugUploadRequestURL provides a mock function with given fields: url
func (_m *Flags) SetDebugUploadRequestURL(url string) error {
	ret := _m.Called(url)

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(url)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetDesktopEnabled provides a mock function with given fields: enabled
func (_m *Flags) SetDesktopEnabled(enabled bool) error {
	ret := _m.Called(enabled)

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

	var r0 error
	if rf, ok := ret.Get(0).(func(bool) error); ok {
		r0 = rf(enabled)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetForceControlSubsystems provides a mock function with given fields: force
func (_m *Flags) SetForceControlSubsystems(force bool) error {
	ret := _m.Called(force)

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

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(url)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetLogIngestServerURL provides a mock function with given fields: url
func (_m *Flags) SetLogIngestServerURL(url string) error {
	ret := _m.Called(url)

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(url)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetLoggingInterval provides a mock function with given fields: interval
func (_m *Flags) SetLoggingInterval(interval time.Duration) error {
	ret := _m.Called(interval)

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

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(url)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetNotaryPrefix provides a mock function with given fields: prefix
func (_m *Flags) SetNotaryPrefix(prefix string) error {
	ret := _m.Called(prefix)

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(prefix)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetNotaryServerURL provides a mock function with given fields: url
func (_m *Flags) SetNotaryServerURL(url string) error {
	ret := _m.Called(url)

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

	var r0 error
	if rf, ok := ret.Get(0).(func(bool) error); ok {
		r0 = rf(verbose)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetTraceIngestServerURL provides a mock function with given fields: url
func (_m *Flags) SetTraceIngestServerURL(url string) error {
	ret := _m.Called(url)

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

	var r0 error
	if rf, ok := ret.Get(0).(func(float64) error); ok {
		r0 = rf(rate)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetTufServerURL provides a mock function with given fields: url
func (_m *Flags) SetTufServerURL(url string) error {
	ret := _m.Called(url)

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

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(directory)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// TraceIngestServerURL provides a mock function with given fields:
func (_m *Flags) TraceIngestServerURL() string {
	ret := _m.Called()

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

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
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
