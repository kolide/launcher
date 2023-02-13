// Code generated by mockery v2.20.0. DO NOT EDIT.

package mocks

import mock "github.com/stretchr/testify/mock"

// Querier is an autogenerated mock type for the Querier type
type Querier struct {
	mock.Mock
}

// Query provides a mock function with given fields: query
func (_m *Querier) Query(query string) ([]map[string]string, error) {
	ret := _m.Called(query)

	var r0 []map[string]string
	var r1 error
	if rf, ok := ret.Get(0).(func(string) ([]map[string]string, error)); ok {
		return rf(query)
	}
	if rf, ok := ret.Get(0).(func(string) []map[string]string); ok {
		r0 = rf(query)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]map[string]string)
		}
	}

	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(query)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
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
