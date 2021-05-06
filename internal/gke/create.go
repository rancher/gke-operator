package gke

import (
	"context"
	"fmt"
	"strings"

	gkev1 "github.com/rancher/gke-operator/pkg/apis/gke.cattle.io/v1"
	gkeapi "google.golang.org/api/container/v1"
)

// Errors
const (
	cannotBeNilError            = "field [%s] cannot be nil for non-import cluster [%s]"
	cannotBeNilForNodePoolError = "field [%s] cannot be nil for nodepool [%s] in non-nil cluster [%s]"
)

// Create creates an upstream GKE cluster.
func Create(ctx context.Context, client *gkeapi.Service, config *gkev1.GKEClusterConfig) error {
	err := validateCreateRequest(ctx, client, config)
	if err != nil {
		return err
	}

	createClusterRequest := newClusterCreateRequest(config)

	_, err = client.Projects.
		Locations.
		Clusters.
		Create(
			LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone)),
			createClusterRequest).
		Context(ctx).
		Do()

	return err
}

// CreateNodePool creates an upstream node pool with the given cluster as a parent.
func CreateNodePool(ctx context.Context, client *gkeapi.Service, config *gkev1.GKEClusterConfig, nodePoolConfig *gkev1.GKENodePoolConfig) (Status, error) {
	err := validateNodePoolCreateRequest(config.Spec.ClusterName, nodePoolConfig)
	if err != nil {
		return NotChanged, err
	}

	createNodePoolRequest, err := newNodePoolCreateRequest(
		ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName),
		nodePoolConfig,
	)
	if err != nil {
		return NotChanged, err
	}

	_, err = client.Projects.
		Locations.
		Clusters.
		NodePools.
		Create(
			ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName),
			createNodePoolRequest,
		).Context(ctx).Do()
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

	addons := config.Spec.ClusterAddons
	request.Cluster.AddonsConfig.HttpLoadBalancing = &gkeapi.HttpLoadBalancing{Disabled: !addons.HTTPLoadBalancing}
	request.Cluster.AddonsConfig.HorizontalPodAutoscaling = &gkeapi.HorizontalPodAutoscaling{Disabled: !addons.HorizontalPodAutoscaling}
	request.Cluster.AddonsConfig.NetworkPolicyConfig = &gkeapi.NetworkPolicyConfig{Disabled: !addons.NetworkPolicyConfig}

	request.Cluster.NodePools = make([]*gkeapi.NodePool, 0, len(config.Spec.NodePools))

	for _, np := range config.Spec.NodePools {
		nodePool := newGKENodePoolFromConfig(&np)
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

	if config.Spec.PrivateClusterConfig != nil {
		request.Cluster.PrivateClusterConfig = &gkeapi.PrivateClusterConfig{
			EnablePrivateEndpoint: config.Spec.PrivateClusterConfig.EnablePrivateEndpoint,
			EnablePrivateNodes:    config.Spec.PrivateClusterConfig.EnablePrivateNodes,
			MasterIpv4CidrBlock:   config.Spec.PrivateClusterConfig.MasterIpv4CidrBlock,
		}
	}

	return request
}

// validateCreateRequest checks a config for the ability to generate a create request
func validateCreateRequest(ctx context.Context, client *gkeapi.Service, config *gkev1.GKEClusterConfig) error {
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

	for _, np := range config.Spec.NodePools {
		if np.Autoscaling != nil && np.Autoscaling.Enabled {
			if np.Autoscaling.MinNodeCount < 1 || np.Autoscaling.MaxNodeCount < np.Autoscaling.MinNodeCount {
				return fmt.Errorf("minNodeCount in the NodePool must be >= 1 and <= maxNodeCount")
			}
		}
	}

	operation, err := client.Projects.
		Locations.
		Clusters.
		List(LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone))).
		Context(ctx).
		Do()
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

	emptyString := ""
	if config.Spec.EnableKubernetesAlpha == nil {
		return fmt.Errorf(cannotBeNilError, "enableKubernetesAlpha", config.Name)
	}
	if config.Spec.KubernetesVersion == nil {
		return fmt.Errorf(cannotBeNilError, "kubernetesVersion", config.Name)
	}
	if config.Spec.ClusterIpv4CidrBlock == nil {
		config.Spec.ClusterIpv4CidrBlock = &emptyString
	}
	if config.Spec.ClusterAddons == nil {
		config.Spec.ClusterAddons = &gkev1.GKEClusterAddons{}
	}
	if config.Spec.IPAllocationPolicy == nil {
		config.Spec.IPAllocationPolicy = &gkev1.GKEIPAllocationPolicy{}
	}
	if config.Spec.LoggingService == nil {
		config.Spec.LoggingService = &emptyString
	}
	if config.Spec.Network == nil {
		config.Spec.Network = &emptyString
	}
	if config.Spec.Subnetwork == nil {
		config.Spec.Subnetwork = &emptyString
	}
	if config.Spec.NetworkPolicyEnabled == nil {
		return fmt.Errorf(cannotBeNilError, "networkPolicyEnabled", config.Name)
	}
	if config.Spec.PrivateClusterConfig == nil {
		config.Spec.PrivateClusterConfig = &gkev1.GKEPrivateClusterConfig{}
	}
	if config.Spec.MasterAuthorizedNetworksConfig == nil {
		config.Spec.MasterAuthorizedNetworksConfig = &gkev1.GKEMasterAuthorizedNetworksConfig{}
	}
	if config.Spec.MonitoringService == nil {
		config.Spec.MonitoringService = &emptyString
	}
	if config.Spec.Locations == nil {
		config.Spec.Locations = []string{}
	}
	if config.Spec.MaintenanceWindow == nil {
		config.Spec.MaintenanceWindow = &emptyString
	}
	if config.Spec.Labels == nil {
		config.Spec.Labels = map[string]string{}
	}

	for _, np := range config.Spec.NodePools {
		if err = validateNodePoolCreateRequest(config.Spec.ClusterName, &np); err != nil {
			return err
		}
	}

	return nil
}

func validateNodePoolCreateRequest(clusterName string, np *gkev1.GKENodePoolConfig) error {
	clusterErr := cannotBeNilError
	nodePoolErr := cannotBeNilForNodePoolError
	if np.Name == nil {
		return fmt.Errorf(clusterErr, "nodePool.name", clusterName)
	}
	if np.Version == nil {
		return fmt.Errorf(nodePoolErr, "version", *np.Name, clusterName)
	}
	if np.Autoscaling == nil {
		np.Autoscaling = &gkev1.GKENodePoolAutoscaling{}
	}
	if np.InitialNodeCount == nil {
		return fmt.Errorf(nodePoolErr, "initialNodeCount", *np.Name, clusterName)
	}
	if np.MaxPodsConstraint == nil {
		return fmt.Errorf(nodePoolErr, "maxPodsConstraint", *np.Name, clusterName)
	}
	if np.Config == nil {
		np.Config = &gkev1.GKENodeConfig{}
	}
	if np.Management == nil {
		np.Management = &gkev1.GKENodePoolManagement{}
	}
	return nil
}

func newNodePoolCreateRequest(parent string, np *gkev1.GKENodePoolConfig) (*gkeapi.CreateNodePoolRequest, error) {
	request := &gkeapi.CreateNodePoolRequest{
		Parent:   parent,
		NodePool: newGKENodePoolFromConfig(np),
	}
	return request, nil
}

func newGKENodePoolFromConfig(np *gkev1.GKENodePoolConfig) *gkeapi.NodePool {
	taints := make([]*gkeapi.NodeTaint, 0, len(np.Config.Taints))
	for _, t := range np.Config.Taints {
		taints = append(taints, &gkeapi.NodeTaint{
			Effect: t.Effect,
			Key:    t.Key,
			Value:  t.Value,
		})
	}
	return &gkeapi.NodePool{
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
			Taints:        taints,
		},
		MaxPodsConstraint: &gkeapi.MaxPodsConstraint{
			MaxPodsPerNode: *np.MaxPodsConstraint,
		},
		Version: *np.Version,
		Management: &gkeapi.NodeManagement{
			AutoRepair:  np.Management.AutoRepair,
			AutoUpgrade: np.Management.AutoUpgrade,
		},
	}
}
