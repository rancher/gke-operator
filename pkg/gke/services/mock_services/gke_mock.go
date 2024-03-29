// Code generated by MockGen. DO NOT EDIT.
// Source: ../gke.go

// Package mock_services is a generated GoMock package.
package mock_services

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	container "google.golang.org/api/container/v1"
)

// MockGKEClusterService is a mock of GKEClusterService interface.
type MockGKEClusterService struct {
	ctrl     *gomock.Controller
	recorder *MockGKEClusterServiceMockRecorder
}

// MockGKEClusterServiceMockRecorder is the mock recorder for MockGKEClusterService.
type MockGKEClusterServiceMockRecorder struct {
	mock *MockGKEClusterService
}

// NewMockGKEClusterService creates a new mock instance.
func NewMockGKEClusterService(ctrl *gomock.Controller) *MockGKEClusterService {
	mock := &MockGKEClusterService{ctrl: ctrl}
	mock.recorder = &MockGKEClusterServiceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockGKEClusterService) EXPECT() *MockGKEClusterServiceMockRecorder {
	return m.recorder
}

// ClusterCreate mocks base method.
func (m *MockGKEClusterService) ClusterCreate(ctx context.Context, parent string, createclusterrequest *container.CreateClusterRequest) (*container.Operation, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ClusterCreate", ctx, parent, createclusterrequest)
	ret0, _ := ret[0].(*container.Operation)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ClusterCreate indicates an expected call of ClusterCreate.
func (mr *MockGKEClusterServiceMockRecorder) ClusterCreate(ctx, parent, createclusterrequest interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ClusterCreate", reflect.TypeOf((*MockGKEClusterService)(nil).ClusterCreate), ctx, parent, createclusterrequest)
}

// ClusterDelete mocks base method.
func (m *MockGKEClusterService) ClusterDelete(ctx context.Context, name string) (*container.Operation, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ClusterDelete", ctx, name)
	ret0, _ := ret[0].(*container.Operation)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ClusterDelete indicates an expected call of ClusterDelete.
func (mr *MockGKEClusterServiceMockRecorder) ClusterDelete(ctx, name interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ClusterDelete", reflect.TypeOf((*MockGKEClusterService)(nil).ClusterDelete), ctx, name)
}

// ClusterGet mocks base method.
func (m *MockGKEClusterService) ClusterGet(ctx context.Context, name string) (*container.Cluster, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ClusterGet", ctx, name)
	ret0, _ := ret[0].(*container.Cluster)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ClusterGet indicates an expected call of ClusterGet.
func (mr *MockGKEClusterServiceMockRecorder) ClusterGet(ctx, name interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ClusterGet", reflect.TypeOf((*MockGKEClusterService)(nil).ClusterGet), ctx, name)
}

// ClusterList mocks base method.
func (m *MockGKEClusterService) ClusterList(ctx context.Context, parent string) (*container.ListClustersResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ClusterList", ctx, parent)
	ret0, _ := ret[0].(*container.ListClustersResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ClusterList indicates an expected call of ClusterList.
func (mr *MockGKEClusterServiceMockRecorder) ClusterList(ctx, parent interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ClusterList", reflect.TypeOf((*MockGKEClusterService)(nil).ClusterList), ctx, parent)
}

// ClusterUpdate mocks base method.
func (m *MockGKEClusterService) ClusterUpdate(ctx context.Context, name string, updateclusterrequest *container.UpdateClusterRequest) (*container.Operation, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ClusterUpdate", ctx, name, updateclusterrequest)
	ret0, _ := ret[0].(*container.Operation)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ClusterUpdate indicates an expected call of ClusterUpdate.
func (mr *MockGKEClusterServiceMockRecorder) ClusterUpdate(ctx, name, updateclusterrequest interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ClusterUpdate", reflect.TypeOf((*MockGKEClusterService)(nil).ClusterUpdate), ctx, name, updateclusterrequest)
}

// NodePoolCreate mocks base method.
func (m *MockGKEClusterService) NodePoolCreate(ctx context.Context, parent string, createnodepoolrequest *container.CreateNodePoolRequest) (*container.Operation, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NodePoolCreate", ctx, parent, createnodepoolrequest)
	ret0, _ := ret[0].(*container.Operation)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// NodePoolCreate indicates an expected call of NodePoolCreate.
func (mr *MockGKEClusterServiceMockRecorder) NodePoolCreate(ctx, parent, createnodepoolrequest interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NodePoolCreate", reflect.TypeOf((*MockGKEClusterService)(nil).NodePoolCreate), ctx, parent, createnodepoolrequest)
}

// NodePoolDelete mocks base method.
func (m *MockGKEClusterService) NodePoolDelete(ctx context.Context, name string) (*container.Operation, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NodePoolDelete", ctx, name)
	ret0, _ := ret[0].(*container.Operation)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// NodePoolDelete indicates an expected call of NodePoolDelete.
func (mr *MockGKEClusterServiceMockRecorder) NodePoolDelete(ctx, name interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NodePoolDelete", reflect.TypeOf((*MockGKEClusterService)(nil).NodePoolDelete), ctx, name)
}

// NodePoolGet mocks base method.
func (m *MockGKEClusterService) NodePoolGet(ctx context.Context, name string) (*container.NodePool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NodePoolGet", ctx, name)
	ret0, _ := ret[0].(*container.NodePool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// NodePoolGet indicates an expected call of NodePoolGet.
func (mr *MockGKEClusterServiceMockRecorder) NodePoolGet(ctx, name interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NodePoolGet", reflect.TypeOf((*MockGKEClusterService)(nil).NodePoolGet), ctx, name)
}

// NodePoolList mocks base method.
func (m *MockGKEClusterService) NodePoolList(ctx context.Context, parent string) (*container.ListNodePoolsResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NodePoolList", ctx, parent)
	ret0, _ := ret[0].(*container.ListNodePoolsResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// NodePoolList indicates an expected call of NodePoolList.
func (mr *MockGKEClusterServiceMockRecorder) NodePoolList(ctx, parent interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NodePoolList", reflect.TypeOf((*MockGKEClusterService)(nil).NodePoolList), ctx, parent)
}

// NodePoolUpdate mocks base method.
func (m *MockGKEClusterService) NodePoolUpdate(ctx context.Context, name string, updatenodepoolrequest *container.UpdateNodePoolRequest) (*container.Operation, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NodePoolUpdate", ctx, name, updatenodepoolrequest)
	ret0, _ := ret[0].(*container.Operation)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// NodePoolUpdate indicates an expected call of NodePoolUpdate.
func (mr *MockGKEClusterServiceMockRecorder) NodePoolUpdate(ctx, name, updatenodepoolrequest interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NodePoolUpdate", reflect.TypeOf((*MockGKEClusterService)(nil).NodePoolUpdate), ctx, name, updatenodepoolrequest)
}

// SetAutoscaling mocks base method.
func (m *MockGKEClusterService) SetAutoscaling(ctx context.Context, name string, setnodepoolautoscalingrequest *container.SetNodePoolAutoscalingRequest) (*container.Operation, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetAutoscaling", ctx, name, setnodepoolautoscalingrequest)
	ret0, _ := ret[0].(*container.Operation)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// SetAutoscaling indicates an expected call of SetAutoscaling.
func (mr *MockGKEClusterServiceMockRecorder) SetAutoscaling(ctx, name, setnodepoolautoscalingrequest interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetAutoscaling", reflect.TypeOf((*MockGKEClusterService)(nil).SetAutoscaling), ctx, name, setnodepoolautoscalingrequest)
}

// SetMaintenancePolicy mocks base method.
func (m *MockGKEClusterService) SetMaintenancePolicy(ctx context.Context, name string, maintenancepolicyrequest *container.SetMaintenancePolicyRequest) (*container.Operation, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetMaintenancePolicy", ctx, name, maintenancepolicyrequest)
	ret0, _ := ret[0].(*container.Operation)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// SetMaintenancePolicy indicates an expected call of SetMaintenancePolicy.
func (mr *MockGKEClusterServiceMockRecorder) SetMaintenancePolicy(ctx, name, maintenancepolicyrequest interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetMaintenancePolicy", reflect.TypeOf((*MockGKEClusterService)(nil).SetMaintenancePolicy), ctx, name, maintenancepolicyrequest)
}

// SetManagement mocks base method.
func (m *MockGKEClusterService) SetManagement(ctx context.Context, name string, setnodepoolmanagementrequest *container.SetNodePoolManagementRequest) (*container.Operation, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetManagement", ctx, name, setnodepoolmanagementrequest)
	ret0, _ := ret[0].(*container.Operation)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// SetManagement indicates an expected call of SetManagement.
func (mr *MockGKEClusterServiceMockRecorder) SetManagement(ctx, name, setnodepoolmanagementrequest interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetManagement", reflect.TypeOf((*MockGKEClusterService)(nil).SetManagement), ctx, name, setnodepoolmanagementrequest)
}

// SetNetworkPolicy mocks base method.
func (m *MockGKEClusterService) SetNetworkPolicy(ctx context.Context, name string, networkpolicyrequest *container.SetNetworkPolicyRequest) (*container.Operation, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetNetworkPolicy", ctx, name, networkpolicyrequest)
	ret0, _ := ret[0].(*container.Operation)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// SetNetworkPolicy indicates an expected call of SetNetworkPolicy.
func (mr *MockGKEClusterServiceMockRecorder) SetNetworkPolicy(ctx, name, networkpolicyrequest interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetNetworkPolicy", reflect.TypeOf((*MockGKEClusterService)(nil).SetNetworkPolicy), ctx, name, networkpolicyrequest)
}

// SetResourceLabels mocks base method.
func (m *MockGKEClusterService) SetResourceLabels(ctx context.Context, name string, resourcelabelsrequest *container.SetLabelsRequest) (*container.Operation, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetResourceLabels", ctx, name, resourcelabelsrequest)
	ret0, _ := ret[0].(*container.Operation)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// SetResourceLabels indicates an expected call of SetResourceLabels.
func (mr *MockGKEClusterServiceMockRecorder) SetResourceLabels(ctx, name, resourcelabelsrequest interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetResourceLabels", reflect.TypeOf((*MockGKEClusterService)(nil).SetResourceLabels), ctx, name, resourcelabelsrequest)
}

// SetSize mocks base method.
func (m *MockGKEClusterService) SetSize(ctx context.Context, name string, setnodepoolsizerequest *container.SetNodePoolSizeRequest) (*container.Operation, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetSize", ctx, name, setnodepoolsizerequest)
	ret0, _ := ret[0].(*container.Operation)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// SetSize indicates an expected call of SetSize.
func (mr *MockGKEClusterServiceMockRecorder) SetSize(ctx, name, setnodepoolsizerequest interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetSize", reflect.TypeOf((*MockGKEClusterService)(nil).SetSize), ctx, name, setnodepoolsizerequest)
}
