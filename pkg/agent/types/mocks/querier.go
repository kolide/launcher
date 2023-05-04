// Code generated by mockery v2.23.1. DO NOT EDIT.

package mocks

import mock "github.com/stretchr/testify/mock"

// Querier is an autogenerated mock type for the Querier type
type Querier struct {
	mock.Mock
}

// Query provides a mock function with given fields: query, callback
func (_m *Querier) Query(query string, callback func([]map[string]string, error)) error {
	ret := _m.Called(query, callback)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, func([]map[string]string, error)) error); ok {
		r0 = rf(query, callback)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

type mockConstructorTestingTNewQuerier interface {
	mock.TestingT
	Cleanup(func())
}

// NewQuerier creates a new instance of Querier. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewQuerier(t mockConstructorTestingTNewQuerier) *Querier {
	mock := &Querier{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
