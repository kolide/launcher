// Code generated by mockery v2.21.1. DO NOT EDIT.

package mocks

import (
	tuf "github.com/kolide/updater/tuf"
	mock "github.com/stretchr/testify/mock"
)

// Updater is an autogenerated mock type for the updater type
type Updater struct {
	mock.Mock
}

// ErrorCount provides a mock function with given fields:
func (_m *Updater) ErrorCount() int {
	ret := _m.Called()

	var r0 int
	if rf, ok := ret.Get(0).(func() int); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(int)
	}

	return r0
}

// Run provides a mock function with given fields: opts
func (_m *Updater) Run(opts ...tuf.Option) (func(), error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 func()
	var r1 error
	if rf, ok := ret.Get(0).(func(...tuf.Option) (func(), error)); ok {
		return rf(opts...)
	}
	if rf, ok := ret.Get(0).(func(...tuf.Option) func()); ok {
		r0 = rf(opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(func())
		}
	}

	if rf, ok := ret.Get(1).(func(...tuf.Option) error); ok {
		r1 = rf(opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

type mockConstructorTestingTNewUpdater interface {
	mock.TestingT
	Cleanup(func())
}

// NewUpdater creates a new instance of Updater. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewUpdater(t mockConstructorTestingTNewUpdater) *Updater {
	mock := &Updater{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
