// Code generated by mockery v2.20.0. DO NOT EDIT.

package mocks

import (
	time "time"

	mock "github.com/stretchr/testify/mock"
)

// ControlService is an autogenerated mock type for the controlService type
type ControlService struct {
	mock.Mock
}

// AccelerateRequestInterval provides a mock function with given fields: interval, duration
func (_m *ControlService) AccelerateRequestInterval(interval time.Duration, duration time.Duration) {
	_m.Called(interval, duration)
}

type mockConstructorTestingTNewControlService interface {
	mock.TestingT
	Cleanup(func())
}

// NewControlService creates a new instance of ControlService. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewControlService(t mockConstructorTestingTNewControlService) *ControlService {
	mock := &ControlService{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
