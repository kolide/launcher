// Code generated by mockery v2.45.0. DO NOT EDIT.

package mocks

import (
	context "context"

	io "io"

	mock "github.com/stretchr/testify/mock"
)

// DataProvider is an autogenerated mock type for the dataProvider type
type DataProvider struct {
	mock.Mock
}

// GetConfig provides a mock function with given fields: ctx
func (_m *DataProvider) GetConfig(ctx context.Context) (io.Reader, error) {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for GetConfig")
	}

	var r0 io.Reader
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context) (io.Reader, error)); ok {
		return rf(ctx)
	}
	if rf, ok := ret.Get(0).(func(context.Context) io.Reader); ok {
		r0 = rf(ctx)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(io.Reader)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(ctx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetSubsystemData provides a mock function with given fields: ctx, hash
func (_m *DataProvider) GetSubsystemData(ctx context.Context, hash string) (io.Reader, error) {
	ret := _m.Called(ctx, hash)

	if len(ret) == 0 {
		panic("no return value specified for GetSubsystemData")
	}

	var r0 io.Reader
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string) (io.Reader, error)); ok {
		return rf(ctx, hash)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) io.Reader); ok {
		r0 = rf(ctx, hash)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(io.Reader)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, hash)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// SendMessage provides a mock function with given fields: ctx, method, params
func (_m *DataProvider) SendMessage(ctx context.Context, method string, params interface{}) error {
	ret := _m.Called(ctx, method, params)

	if len(ret) == 0 {
		panic("no return value specified for SendMessage")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, interface{}) error); ok {
		r0 = rf(ctx, method, params)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// newDataProvider creates a new instance of dataProvider. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewDataProvider(t interface {
	mock.TestingT
	Cleanup(func())
}) *DataProvider {
	mock := &DataProvider{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
