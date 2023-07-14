package services

import (
	"context"

	"golang.org/x/oauth2"
	gkeapi "google.golang.org/api/container/v1"
	"google.golang.org/api/option"
)

type GKEClusterService interface {
	Create(parent string, cluster *gkeapi.CreateClusterRequest) (*gkeapi.ProjectsLocationsClustersCreateCall, error)
	List(parent string) (*gkeapi.ProjectsLocationsClustersListCall, error)
	Get(name string) (*gkeapi.ProjectsLocationsClustersGetCall, error)
	Delete(name string) (*gkeapi.ProjectsLocationsClustersDeleteCall, error)
	SetNetworkPolicy(name string, networkPolicy *gkeapi.SetNetworkPolicyRequest) (*gkeapi.ProjectsLocationsClustersSetNetworkPolicyCall, error)
	SetMaintenancePolicy(name string, maintenancePolicy *gkeapi.SetMaintenancePolicyRequest) (*gkeapi.ProjectsLocationsClustersSetMaintenancePolicyCall, error)
	SetResourceLabels(name string, resourceLabels *gkeapi.SetLabelsRequest) (*gkeapi.ProjectsLocationsClustersSetResourceLabelsCall, error)
}

type gkeClusterService struct {
	svc *gkeapi.ProjectsLocationsClustersService
}

func NewGKEClusterService(ctx context.Context, ts oauth2.TokenSource) (GKEClusterService, error) {
	svc, err := gkeapi.NewService(ctx, option.WithHTTPClient(oauth2.NewClient(ctx, ts)))
	if err != nil {
		return nil, err
	}
	return &gkeClusterService{
		svc: svc.Projects.Locations.Clusters,
	}, nil
}

func (g *gkeClusterService) Create(parent string, cluster *gkeapi.CreateClusterRequest) (*gkeapi.ProjectsLocationsClustersCreateCall, error) {
	return g.svc.Create(parent, cluster), nil
}

func (g *gkeClusterService) List(parent string) (*gkeapi.ProjectsLocationsClustersListCall, error) {
	return g.svc.List(parent), nil
}

func (g *gkeClusterService) Get(name string) (*gkeapi.ProjectsLocationsClustersGetCall, error) {
	return g.svc.Get(name), nil
}

func (g *gkeClusterService) Delete(name string) (*gkeapi.ProjectsLocationsClustersDeleteCall, error) {
	return g.svc.Delete(name), nil
}

func (g *gkeClusterService) SetNetworkPolicy(name string, networkPolicy *gkeapi.SetNetworkPolicyRequest) (*gkeapi.ProjectsLocationsClustersSetNetworkPolicyCall, error) {
	return g.svc.SetNetworkPolicy(name, networkPolicy), nil
}

func (g *gkeClusterService) SetMaintenancePolicy(name string, maintenancePolicy *gkeapi.SetMaintenancePolicyRequest) (*gkeapi.ProjectsLocationsClustersSetMaintenancePolicyCall, error) {
	return g.svc.SetMaintenancePolicy(name, maintenancePolicy), nil
}

func (g *gkeClusterService) SetResourceLabels(name string, resourceLabels *gkeapi.SetLabelsRequest) (*gkeapi.ProjectsLocationsClustersSetResourceLabelsCall, error) {
	return g.svc.SetResourceLabels(name, resourceLabels), nil
}

type GKENodePoolService interface {
	Create(parent string, cluster *gkeapi.CreateNodePoolRequest) (*gkeapi.ProjectsLocationsClustersNodePoolsCreateCall, error)
	List(parent string) (*gkeapi.ProjectsLocationsClustersNodePoolsListCall, error)
	Get(name string) (*gkeapi.ProjectsLocationsClustersNodePoolsGetCall, error)
	Update(name string, nodepool *gkeapi.UpdateNodePoolRequest) (*gkeapi.ProjectsLocationsClustersNodePoolsUpdateCall, error)
	Delete(name string) (*gkeapi.ProjectsLocationsClustersNodePoolsDeleteCall, error)
	SetSize(name string, size *gkeapi.SetNodePoolSizeRequest) (*gkeapi.ProjectsLocationsClustersNodePoolsSetSizeCall, error)
	SetAutoscaling(name string, autoscaling *gkeapi.SetNodePoolAutoscalingRequest) (*gkeapi.ProjectsLocationsClustersNodePoolsSetAutoscalingCall, error)
	SetManagement(name string, management *gkeapi.SetNodePoolManagementRequest) (*gkeapi.ProjectsLocationsClustersNodePoolsSetManagementCall, error)
}

type gkeNodePoolService struct {
	svc *gkeapi.ProjectsLocationsClustersNodePoolsService
}

func NewGKENodePoolService(ctx context.Context, ts oauth2.TokenSource) (GKENodePoolService, error) {
	svc, err := gkeapi.NewService(ctx, option.WithHTTPClient(oauth2.NewClient(ctx, ts)))
	if err != nil {
		return nil, err
	}
	return &gkeNodePoolService{
		svc: svc.Projects.Locations.Clusters.NodePools,
	}, nil
}

func (g *gkeNodePoolService) Create(parent string, nodepool *gkeapi.CreateNodePoolRequest) (*gkeapi.ProjectsLocationsClustersNodePoolsCreateCall, error) {
	return g.svc.Create(parent, nodepool), nil
}

func (g *gkeNodePoolService) List(parent string) (*gkeapi.ProjectsLocationsClustersNodePoolsListCall, error) {
	return g.svc.List(parent), nil
}

func (g *gkeNodePoolService) Get(name string) (*gkeapi.ProjectsLocationsClustersNodePoolsGetCall, error) {
	return g.svc.Get(name), nil
}

func (g *gkeNodePoolService) Update(name string, nodepool *gkeapi.UpdateNodePoolRequest) (*gkeapi.ProjectsLocationsClustersNodePoolsUpdateCall, error) {
	return g.svc.Update(name, nodepool), nil
}

func (g *gkeNodePoolService) Delete(name string) (*gkeapi.ProjectsLocationsClustersNodePoolsDeleteCall, error) {
	return g.svc.Delete(name), nil
}

func (g *gkeNodePoolService) SetSize(name string, size *gkeapi.SetNodePoolSizeRequest) (*gkeapi.ProjectsLocationsClustersNodePoolsSetSizeCall, error) {
	return g.svc.SetSize(name, size), nil
}

func (g *gkeNodePoolService) SetAutoscaling(name string, autoscaling *gkeapi.SetNodePoolAutoscalingRequest) (*gkeapi.ProjectsLocationsClustersNodePoolsSetAutoscalingCall, error) {
	return g.svc.SetAutoscaling(name, autoscaling), nil
}

func (g *gkeNodePoolService) SetManagement(name string, management *gkeapi.SetNodePoolManagementRequest) (*gkeapi.ProjectsLocationsClustersNodePoolsSetManagementCall, error) {
	return g.svc.SetManagement(name, management), nil
}
