// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/Garetonchick/distbuild/pkg/api (interfaces: HeartbeatService)

// Package mock is a generated GoMock package.
package mock

import (
	context "context"
	gomock "github.com/golang/mock/gomock"
	api "github.com/Garetonchick/distbuild/pkg/api"
	reflect "reflect"
)

// MockHeartbeatService is a mock of HeartbeatService interface
type MockHeartbeatService struct {
	ctrl     *gomock.Controller
	recorder *MockHeartbeatServiceMockRecorder
}

// MockHeartbeatServiceMockRecorder is the mock recorder for MockHeartbeatService
type MockHeartbeatServiceMockRecorder struct {
	mock *MockHeartbeatService
}

// NewMockHeartbeatService creates a new mock instance
func NewMockHeartbeatService(ctrl *gomock.Controller) *MockHeartbeatService {
	mock := &MockHeartbeatService{ctrl: ctrl}
	mock.recorder = &MockHeartbeatServiceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockHeartbeatService) EXPECT() *MockHeartbeatServiceMockRecorder {
	return m.recorder
}

// Heartbeat mocks base method
func (m *MockHeartbeatService) Heartbeat(arg0 context.Context, arg1 *api.HeartbeatRequest) (*api.HeartbeatResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Heartbeat", arg0, arg1)
	ret0, _ := ret[0].(*api.HeartbeatResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Heartbeat indicates an expected call of Heartbeat
func (mr *MockHeartbeatServiceMockRecorder) Heartbeat(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Heartbeat", reflect.TypeOf((*MockHeartbeatService)(nil).Heartbeat), arg0, arg1)
}
