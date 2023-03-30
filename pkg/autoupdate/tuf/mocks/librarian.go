// Code generated by mockery v2.21.1. DO NOT EDIT.

package mocks

import mock "github.com/stretchr/testify/mock"

// Librarian is an autogenerated mock type for the librarian type
type Librarian struct {
	mock.Mock
}

// AddToLibrary provides a mock function with given fields: binary, targetFilename
func (_m *Librarian) AddToLibrary(binary string, targetFilename string) error {
	ret := _m.Called(binary, targetFilename)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string) error); ok {
		r0 = rf(binary, targetFilename)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

type mockConstructorTestingTNewLibrarian interface {
	mock.TestingT
	Cleanup(func())
}

// NewLibrarian creates a new instance of Librarian. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewLibrarian(t mockConstructorTestingTNewLibrarian) *Librarian {
	mock := &Librarian{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
