package gke

import (
	"context"
	"fmt"
	"strings"

	gkev1 "github.com/rancher/gke-operator/pkg/apis/gke.cattle.io/v1"
	"github.com/rancher/gke-operator/pkg/gke/services"
	gkeapi "google.golang.org/api/container/v1"
)

// Errors
const (
	cannotBeNilError            = "field [%s] cannot be nil for non-import cluster [%s]"
	cannotBeNilForNodePoolError = "field [%s] cannot be nil for nodepool [%s] in non-nil cluster [%s]"
)

// Create creates an upstream GKE cluster.
func Create(ctx context.Context, gkeClient services.GKEClusterService, config *gkev1.GKEClusterConfig) error {
	err := validateCreateRequest(ctx, gkeClient, config)
	if err != nil {
		return err
	}

	createClusterRequest := newClusterCreateRequest(config)

	_, err = gkeClient.ClusterCreate(ctx,
		LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone)),
		createClusterRequest)

	return err
}

// CreateNodePool creates an upstream node pool with the given cluster as a parent.
func CreateNodePool(ctx context.Context, gkeClient services.GKEClusterService, config *gkev1.GKEClusterConfig, nodePoolConfig *gkev1.GKENodePoolConfig) (Status, error) {
	err := validateNodePoolCreateRequest(nodePoolConfig, config)
	if err != nil {
		return NotChanged, err
	}

	createNodePoolRequest, err := newNodePoolCreateRequest(
		nodePoolConfig,
		config,
	)
	if err != nil {
		return NotChanged, err
	}

	_, err = gkeClient.NodePoolCreate(ctx,
		ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName),
		createNodePoolRequest)
	if err != nil && strings.Contains(err.Error(), errWait) {
		return Retry, nil
	}
	if err != nil {
		return NotChanged, err
	}

	return Changed, nil
}

// newClusterCreateRequest creates a CreateClusterRequest that can be submitted to GKE
func newClusterCreateRequest(config *gkev1.GKEClusterConfig) *gkeapi.CreateClusterRequest {
	enableKubernetesAlpha := config.Spec.EnableKubernetesAlpha != nil && *config.Spec.EnableKubernetesAlpha
	request := &gkeapi.CreateClusterRequest{
		Cluster: &gkeapi.Cluster{
			Name:                  config.Spec.ClusterName,
			Description:           config.Spec.Description,
			ResourceLabels:        config.Spec.Labels,
			InitialClusterVersion: *config.Spec.KubernetesVersion,
			EnableKubernetesAlpha: enableKubernetesAlpha,
			ClusterIpv4Cidr:       *config.Spec.ClusterIpv4CidrBlock,
			LoggingService:        *config.Spec.LoggingService,
			MonitoringService:     *config.Spec.MonitoringService,
			IpAllocationPolicy: &gkeapi.IPAllocationPolicy{
				ClusterIpv4CidrBlock:       config.Spec.IPAllocationPolicy.ClusterIpv4CidrBlock,
				ClusterSecondaryRangeName:  config.Spec.IPAllocationPolicy.ClusterSecondaryRangeName,
				CreateSubnetwork:           config.Spec.IPAllocationPolicy.CreateSubnetwork,
				NodeIpv4CidrBlock:          config.Spec.IPAllocationPolicy.NodeIpv4CidrBlock,
				ServicesIpv4CidrBlock:      config.Spec.IPAllocationPolicy.ServicesIpv4CidrBlock,
				ServicesSecondaryRangeName: config.Spec.IPAllocationPolicy.ServicesSecondaryRangeName,
				SubnetworkName:             config.Spec.IPAllocationPolicy.SubnetworkName,
				UseIpAliases:               config.Spec.IPAllocationPolicy.UseIPAliases,
			},
			AddonsConfig:      &gkeapi.AddonsConfig{},
			NodePools:         []*gkeapi.NodePool{},
			Locations:         config.Spec.Locations,
			MaintenancePolicy: &gkeapi.MaintenancePolicy{},
		},
	}

	if *config.Spec.MaintenanceWindow != "" {
		request.Cluster.MaintenancePolicy.Window = &gkeapi.MaintenanceWindow{
			DailyMaintenanceWindow: &gkeapi.DailyMaintenanceWindow{
				StartTime: *config.Spec.MaintenanceWindow,
			},
		}
	}

	if config.Spec.AutopilotConfig != nil && config.Spec.AutopilotConfig.Enabled {
		request.Cluster.Autopilot = &gkeapi.Autopilot{
			Enabled: config.Spec.AutopilotConfig.Enabled,
		}
	} else {
		addons := config.Spec.ClusterAddons
		request.Cluster.AddonsConfig.HttpLoadBalancing = &gkeapi.HttpLoadBalancing{Disabled: !addons.HTTPLoadBalancing}
		request.Cluster.AddonsConfig.HorizontalPodAutoscaling = &gkeapi.HorizontalPodAutoscaling{Disabled: !addons.HorizontalPodAutoscaling}
		request.Cluster.AddonsConfig.NetworkPolicyConfig = &gkeapi.NetworkPolicyConfig{Disabled: !addons.NetworkPolicyConfig}

		request.Cluster.NodePools = make([]*gkeapi.NodePool, 0, len(config.Spec.NodePools))

		for np := range config.Spec.NodePools {
			nodePool := newGKENodePoolFromConfig(&config.Spec.NodePools[np], config)
			request.Cluster.NodePools = append(request.Cluster.NodePools, nodePool)
		}

		if config.Spec.MasterAuthorizedNetworksConfig != nil {
			blocks := make([]*gkeapi.CidrBlock, len(config.Spec.MasterAuthorizedNetworksConfig.CidrBlocks))
			for _, b := range config.Spec.MasterAuthorizedNetworksConfig.CidrBlocks {
				blocks = append(blocks, &gkeapi.CidrBlock{
					CidrBlock:   b.CidrBlock,
					DisplayName: b.DisplayName,
				})
			}
			request.Cluster.MasterAuthorizedNetworksConfig = &gkeapi.MasterAuthorizedNetworksConfig{
				Enabled:    config.Spec.MasterAuthorizedNetworksConfig.Enabled,
				CidrBlocks: blocks,
			}
		}
	}

	if config.Spec.Network != nil {
		request.Cluster.Network = *config.Spec.Network
	}
	if config.Spec.Subnetwork != nil {
		request.Cluster.Subnetwork = *config.Spec.Subnetwork
	}

	if config.Spec.NetworkPolicyEnabled != nil {
		request.Cluster.NetworkPolicy = &gkeapi.NetworkPolicy{
			Enabled: *config.Spec.NetworkPolicyEnabled,
		}
	}

	if config.Spec.PrivateClusterConfig != nil && config.Spec.PrivateClusterConfig.EnablePrivateNodes {
		request.Cluster.PrivateClusterConfig = &gkeapi.PrivateClusterConfig{
			EnablePrivateEndpoint: config.Spec.PrivateClusterConfig.EnablePrivateEndpoint,
			EnablePrivateNodes:    config.Spec.PrivateClusterConfig.EnablePrivateNodes,
			MasterIpv4CidrBlock:   config.Spec.PrivateClusterConfig.MasterIpv4CidrBlock,
		}
	}

	return request
}

// validateCreateRequest checks a config for the ability to generate a create request
func validateCreateRequest(ctx context.Context, gkeClient services.GKEClusterService, config *gkev1.GKEClusterConfig) error {
	if config.Spec.ProjectID == "" {
		return fmt.Errorf("project ID is required")
	}
	if config.Spec.Zone == "" && config.Spec.Region == "" {
		return fmt.Errorf("zone or region is required")
	}
	if config.Spec.Zone != "" && config.Spec.Region != "" {
		return fmt.Errorf("only one of zone or region must be specified")
	}
	if config.Spec.ClusterName == "" {
		return fmt.Errorf("cluster name is required")
	}

	if len(config.Spec.NodePools) != 0 && config.Spec.AutopilotConfig != nil && config.Spec.AutopilotConfig.Enabled {
		return fmt.Errorf("cannot create node pools for autopilot clusters")
	}

	nodeP := map[string]bool{}
	for _, np := range config.Spec.NodePools {
		if np.Name == nil {
			return fmt.Errorf(cannotBeNilError, "nodePool.name", config.Name)
		}
		if nodeP[*np.Name] {
			return fmt.Errorf("NodePool names must be unique within the [%s] cluster to avoid duplication", config.Spec.ClusterName)
		}
		nodeP[*np.Name] = true

		if np.Autoscaling != nil && np.Autoscaling.Enabled {
			if np.Autoscaling.MinNodeCount < 1 || np.Autoscaling.MaxNodeCount < np.Autoscaling.MinNodeCount {
				return fmt.Errorf("minNodeCount in the NodePool must be >= 1 and <= maxNodeCount")
			}
		}
	}

	operation, err := gkeClient.ClusterList(
		ctx, LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone)))
	if err != nil {
		return err
	}

	for _, cluster := range operation.Clusters {
		if cluster.Name == config.Spec.ClusterName {
			return fmt.Errorf("cannot create cluster [%s] because a cluster in GKE exists with the same name, please delete and recreate with a different name", config.Spec.ClusterName)
		}
	}

	if config.Spec.Imported {
		// Validation from here on out is for nilable attributes, not required for imported clusters
		return nil
	}

	if config.Spec.EnableKubernetesAlpha == nil {
		return fmt.Errorf(cannotBeNilError, "enableKubernetesAlpha", config.Name)
	}
	if config.Spec.KubernetesVersion == nil {
		return fmt.Errorf(cannotBeNilError, "kubernetesVersion", config.Name)
	}
	if config.Spec.ClusterIpv4CidrBlock == nil {
		return fmt.Errorf(cannotBeNilError, "clusterIpv4CidrBlock", config.Name)
	}
	if config.Spec.ClusterAddons == nil {
		return fmt.Errorf(cannotBeNilError, "clusterAddons", config.Name)
	}
	if config.Spec.IPAllocationPolicy == nil {
		return fmt.Errorf(cannotBeNilError, "ipAllocationPolicy", config.Name)
	}
	if config.Spec.LoggingService == nil {
		return fmt.Errorf(cannotBeNilError, "loggingService", config.Name)
	}
	if config.Spec.Network == nil {
		return fmt.Errorf(cannotBeNilError, "network", config.Name)
	}
	if config.Spec.Subnetwork == nil {
		return fmt.Errorf(cannotBeNilError, "subnetwork", config.Name)
	}
	if config.Spec.NetworkPolicyEnabled == nil {
		return fmt.Errorf(cannotBeNilError, "networkPolicyEnabled", config.Name)
	}
	if config.Spec.PrivateClusterConfig == nil {
		return fmt.Errorf(cannotBeNilError, "privateClusterConfig", config.Name)
	}
	if config.Spec.PrivateClusterConfig.EnablePrivateEndpoint && !config.Spec.PrivateClusterConfig.EnablePrivateNodes {
		return fmt.Errorf("private endpoint requires private nodes for cluster [%s]", config.Name)
	}
	if config.Spec.MasterAuthorizedNetworksConfig == nil {
		return fmt.Errorf(cannotBeNilError, "masterAuthorizedNetworksConfig", config.Name)
	}
	if config.Spec.MonitoringService == nil {
		return fmt.Errorf(cannotBeNilError, "monitoringService", config.Name)
	}
	if config.Spec.Locations == nil {
		return fmt.Errorf(cannotBeNilError, "locations", config.Name)
	}
	if config.Spec.MaintenanceWindow == nil {
		return fmt.Errorf(cannotBeNilError, "maintenanceWindow", config.Name)
	}
	if config.Spec.Labels == nil {
		return fmt.Errorf(cannotBeNilError, "labels", config.Name)
	}

	for np := range config.Spec.NodePools {
		if err = validateNodePoolCreateRequest(&config.Spec.NodePools[np], config); err != nil {
			return err
		}
	}

	return nil
}

func validateNodePoolCreateRequest(np *gkev1.GKENodePoolConfig, config *gkev1.GKEClusterConfig) error {
	clusterErr := cannotBeNilError
	nodePoolErr := cannotBeNilForNodePoolError
	clusterName := config.Spec.ClusterName
	if np.Name == nil {
		return fmt.Errorf(clusterErr, "nodePool.name", clusterName)
	}
	if np.Version == nil {
		return fmt.Errorf(nodePoolErr, "version", *np.Name, clusterName)
	}
	if np.Autoscaling == nil {
		return fmt.Errorf(nodePoolErr, "autoscaling", *np.Name, clusterName)
	}
	if np.InitialNodeCount == nil {
		return fmt.Errorf(nodePoolErr, "initialNodeCount", *np.Name, clusterName)
	}
	if np.MaxPodsConstraint == nil && config.Spec.IPAllocationPolicy != nil && config.Spec.IPAllocationPolicy.UseIPAliases {
		return fmt.Errorf(nodePoolErr, "maxPodsConstraint", *np.Name, clusterName)
	}
	if np.Config == nil {
		return fmt.Errorf(nodePoolErr, "config", *np.Name, clusterName)
	}
	if np.Management == nil {
		return fmt.Errorf(nodePoolErr, "management", *np.Name, clusterName)
	}
	return nil
}

func newNodePoolCreateRequest(np *gkev1.GKENodePoolConfig, config *gkev1.GKEClusterConfig) (*gkeapi.CreateNodePoolRequest, error) {
	parent := ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName)
	request := &gkeapi.CreateNodePoolRequest{
		Parent:   parent,
		NodePool: newGKENodePoolFromConfig(np, config),
	}
	return request, nil
}

func newGKENodePoolFromConfig(np *gkev1.GKENodePoolConfig, config *gkev1.GKEClusterConfig) *gkeapi.NodePool {
	taints := make([]*gkeapi.NodeTaint, 0, len(np.Config.Taints))
	for _, t := range np.Config.Taints {
		taints = append(taints, &gkeapi.NodeTaint{
			Effect: t.Effect,
			Key:    t.Key,
			Value:  t.Value,
		})
	}
	ret := &gkeapi.NodePool{
		Name: *np.Name,
		Autoscaling: &gkeapi.NodePoolAutoscaling{
			Enabled:      np.Autoscaling.Enabled,
			MaxNodeCount: np.Autoscaling.MaxNodeCount,
			MinNodeCount: np.Autoscaling.MinNodeCount,
		},
		InitialNodeCount: *np.InitialNodeCount,
		Config: &gkeapi.NodeConfig{
			DiskSizeGb:    np.Config.DiskSizeGb,
			DiskType:      np.Config.DiskType,
			ImageType:     np.Config.ImageType,
			Labels:        np.Config.Labels,
			LocalSsdCount: np.Config.LocalSsdCount,
			MachineType:   np.Config.MachineType,
			OauthScopes:   np.Config.OauthScopes,
			Preemptible:   np.Config.Preemptible,
			Tags:          np.Config.Tags,
			Taints:        taints,
		},
		Version: *np.Version,
		Management: &gkeapi.NodeManagement{
			AutoRepair:  np.Management.AutoRepair,
			AutoUpgrade: np.Management.AutoUpgrade,
		},
	}
	if config.Spec.IPAllocationPolicy != nil && config.Spec.IPAllocationPolicy.UseIPAliases {
		ret.MaxPodsConstraint = &gkeapi.MaxPodsConstraint{
			MaxPodsPerNode: *np.MaxPodsConstraint,
		}
	}
	return ret
}
