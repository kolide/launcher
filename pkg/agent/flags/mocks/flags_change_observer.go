// Code generated by mockery v2.23.1. DO NOT EDIT.

package mocks

import (
	flags "github.com/kolide/launcher/pkg/agent/flags"
	mock "github.com/stretchr/testify/mock"
)

// FlagsChangeObserver is an autogenerated mock type for the FlagsChangeObserver type
type FlagsChangeObserver struct {
	mock.Mock
}

// FlagsChanged provides a mock function with given fields: keys
func (_m *FlagsChangeObserver) FlagsChanged(keys ...flags.FlagKey) {
	_va := make([]interface{}, len(keys))
	for _i := range keys {
		_va[_i] = keys[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, _va...)
	_m.Called(_ca...)
}

type mockConstructorTestingTNewFlagsChangeObserver interface {
	mock.TestingT
	Cleanup(func())
}

// NewFlagsChangeObserver creates a new instance of FlagsChangeObserver. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewFlagsChangeObserver(t mockConstructorTestingTNewFlagsChangeObserver) *FlagsChangeObserver {
	mock := &FlagsChangeObserver{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
