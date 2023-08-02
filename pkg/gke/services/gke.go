package services

import (
	"context"

	"golang.org/x/oauth2"
	gkeapi "google.golang.org/api/container/v1"
	"google.golang.org/api/option"
)

type GKEClusterService interface {
	ClusterCreate(ctx context.Context, parent string, createclusterrequest *gkeapi.CreateClusterRequest) (*gkeapi.Operation, error)
	ClusterList(ctx context.Context, parent string) (*gkeapi.ListClustersResponse, error)
	ClusterGet(ctx context.Context, name string) (*gkeapi.Cluster, error)
	ClusterUpdate(ctx context.Context, name string, updateclusterrequest *gkeapi.UpdateClusterRequest) (*gkeapi.Operation, error)
	ClusterDelete(ctx context.Context, name string) (*gkeapi.Operation, error)
	SetNetworkPolicy(ctx context.Context, name string, networkpolicyrequest *gkeapi.SetNetworkPolicyRequest) (*gkeapi.Operation, error)
	SetMaintenancePolicy(ctx context.Context, name string, maintenancepolicyrequest *gkeapi.SetMaintenancePolicyRequest) (*gkeapi.Operation, error)
	SetResourceLabels(ctx context.Context, name string, resourcelabelsrequest *gkeapi.SetLabelsRequest) (*gkeapi.Operation, error)
	NodePoolCreate(ctx context.Context, parent string, createnodepoolrequest *gkeapi.CreateNodePoolRequest) (*gkeapi.Operation, error)
	NodePoolList(ctx context.Context, parent string) (*gkeapi.ListNodePoolsResponse, error)
	NodePoolGet(ctx context.Context, name string) (*gkeapi.NodePool, error)
	NodePoolUpdate(ctx context.Context, name string, updatenodepoolrequest *gkeapi.UpdateNodePoolRequest) (*gkeapi.Operation, error)
	NodePoolDelete(ctx context.Context, name string) (*gkeapi.Operation, error)
	SetSize(ctx context.Context, name string, setnodepoolsizerequest *gkeapi.SetNodePoolSizeRequest) (*gkeapi.Operation, error)
	SetAutoscaling(ctx context.Context, name string, setnodepoolautoscalingrequest *gkeapi.SetNodePoolAutoscalingRequest) (*gkeapi.Operation, error)
	SetManagement(ctx context.Context, name string, setnodepoolmanagementrequest *gkeapi.SetNodePoolManagementRequest) (*gkeapi.Operation, error)
}

type gkeClusterService struct {
	svc gkeapi.Service
}

func NewGKEClusterService(ctx context.Context, ts oauth2.TokenSource) (GKEClusterService, error) {
	svc, err := gkeapi.NewService(ctx, option.WithHTTPClient(oauth2.NewClient(ctx, ts)))
	if err != nil {
		return nil, err
	}
	return &gkeClusterService{
		svc: *svc,
	}, nil
}

func (g *gkeClusterService) ClusterCreate(ctx context.Context, parent string, createclusterrequest *gkeapi.CreateClusterRequest) (*gkeapi.Operation, error) {
	return g.svc.Projects.Locations.Clusters.Create(parent, createclusterrequest).Context(ctx).Do()
}

func (g *gkeClusterService) ClusterList(ctx context.Context, parent string) (*gkeapi.ListClustersResponse, error) {
	return g.svc.Projects.Locations.Clusters.List(parent).Context(ctx).Do()
}

func (g *gkeClusterService) ClusterGet(ctx context.Context, name string) (*gkeapi.Cluster, error) {
	return g.svc.Projects.Locations.Clusters.Get(name).Context(ctx).Do()
}

func (g *gkeClusterService) ClusterUpdate(ctx context.Context, name string, updateclusterrequest *gkeapi.UpdateClusterRequest) (*gkeapi.Operation, error) {
	return g.svc.Projects.Locations.Clusters.Update(name, updateclusterrequest).Context(ctx).Do()
}

func (g *gkeClusterService) ClusterDelete(ctx context.Context, name string) (*gkeapi.Operation, error) {
	return g.svc.Projects.Locations.Clusters.Delete(name).Context(ctx).Do()
}

func (g *gkeClusterService) SetNetworkPolicy(ctx context.Context, name string, networkpolicyrequest *gkeapi.SetNetworkPolicyRequest) (*gkeapi.Operation, error) {
	return g.svc.Projects.Locations.Clusters.SetNetworkPolicy(name, networkpolicyrequest).Context(ctx).Do()
}

func (g *gkeClusterService) SetMaintenancePolicy(ctx context.Context, name string, maintenancepolicyrequest *gkeapi.SetMaintenancePolicyRequest) (*gkeapi.Operation, error) {
	return g.svc.Projects.Locations.Clusters.SetMaintenancePolicy(name, maintenancepolicyrequest).Context(ctx).Do()
}

func (g *gkeClusterService) SetResourceLabels(ctx context.Context, name string, resourcelabelsrequest *gkeapi.SetLabelsRequest) (*gkeapi.Operation, error) {
	return g.svc.Projects.Locations.Clusters.SetResourceLabels(name, resourcelabelsrequest).Context(ctx).Do()
}

func (g *gkeClusterService) NodePoolCreate(ctx context.Context, parent string, createnodepoolrequest *gkeapi.CreateNodePoolRequest) (*gkeapi.Operation, error) {
	return g.svc.Projects.Locations.Clusters.NodePools.Create(parent, createnodepoolrequest).Context(ctx).Do()
}

func (g *gkeClusterService) NodePoolList(ctx context.Context, parent string) (*gkeapi.ListNodePoolsResponse, error) {
	return g.svc.Projects.Locations.Clusters.NodePools.List(parent).Context(ctx).Do()
}

func (g *gkeClusterService) NodePoolGet(ctx context.Context, name string) (*gkeapi.NodePool, error) {
	return g.svc.Projects.Locations.Clusters.NodePools.Get(name).Context(ctx).Do()
}

func (g *gkeClusterService) NodePoolUpdate(ctx context.Context, name string, updatenodepoolrequest *gkeapi.UpdateNodePoolRequest) (*gkeapi.Operation, error) {
	return g.svc.Projects.Locations.Clusters.NodePools.Update(name, updatenodepoolrequest).Context(ctx).Do()
}

func (g *gkeClusterService) NodePoolDelete(ctx context.Context, name string) (*gkeapi.Operation, error) {
	return g.svc.Projects.Locations.Clusters.NodePools.Delete(name).Context(ctx).Do()
}

func (g *gkeClusterService) SetSize(ctx context.Context, name string, setnodepoolsizerequest *gkeapi.SetNodePoolSizeRequest) (*gkeapi.Operation, error) {
	return g.svc.Projects.Locations.Clusters.NodePools.SetSize(name, setnodepoolsizerequest).Context(ctx).Do()
}

func (g *gkeClusterService) SetAutoscaling(ctx context.Context, name string, setnodepoolautoscalingrequest *gkeapi.SetNodePoolAutoscalingRequest) (*gkeapi.Operation, error) {
	return g.svc.Projects.Locations.Clusters.NodePools.SetAutoscaling(name, setnodepoolautoscalingrequest).Context(ctx).Do()
}

func (g *gkeClusterService) SetManagement(ctx context.Context, name string, setnodepoolmanagementrequest *gkeapi.SetNodePoolManagementRequest) (*gkeapi.Operation, error) {
	return g.svc.Projects.Locations.Clusters.NodePools.SetManagement(name, setnodepoolmanagementrequest).Context(ctx).Do()
}
