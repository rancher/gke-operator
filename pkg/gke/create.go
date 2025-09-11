package gke

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	gkeapi "google.golang.org/api/container/v1"

	gkev1 "github.com/rancher/gke-operator/pkg/apis/gke.cattle.io/v1"
	"github.com/rancher/gke-operator/pkg/gke/services"
	"github.com/rancher/gke-operator/pkg/utils"
)

// Errors
const (
	cannotBeNilError            = "field [%s] cannot be nil for non-import cluster [%s (id: %s)]"
	cannotBeNilForNodePoolError = "field [%s] cannot be nil for nodepool [%s] in non-nil cluster [%s (id: %s)]"
)

// Create creates an upstream GKE cluster.
func Create(ctx context.Context, gkeClient services.GKEClusterService, config *gkev1.GKEClusterConfig) error {
	err := validateCreateRequest(ctx, gkeClient, config)
	if err != nil {
		return err
	}

	createClusterRequest := NewClusterCreateRequest(config)

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

// NewClusterCreateRequest creates a CreateClusterRequest that can be submitted to GKE
func NewClusterCreateRequest(config *gkev1.GKEClusterConfig) *gkeapi.CreateClusterRequest {
	enableKubernetesAlpha := config.Spec.EnableKubernetesAlpha != nil && *config.Spec.EnableKubernetesAlpha
	var clusterIpv4Cidr string

	// Set top-level ClusterIpv4Cidr if specified (validation ensures no conflict with secondary ranges)
	if config.Spec.ClusterIpv4CidrBlock != nil {
		clusterIpv4Cidr = *config.Spec.ClusterIpv4CidrBlock
	}

	request := &gkeapi.CreateClusterRequest{
		Cluster: &gkeapi.Cluster{
			Name:                  config.Spec.ClusterName,
			Description:           config.Spec.Description,
			ResourceLabels:        config.Spec.Labels,
			InitialClusterVersion: *config.Spec.KubernetesVersion,
			EnableKubernetesAlpha: enableKubernetesAlpha,
			ClusterIpv4Cidr:       clusterIpv4Cidr,
			LoggingService:        *config.Spec.LoggingService,
			MonitoringService:     *config.Spec.MonitoringService,
			Locations:             config.Spec.Locations,
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
		},
	}

	// Maintenance Window Configuration
	if config.Spec.MaintenanceWindow != nil && *config.Spec.MaintenanceWindow != "" {
		request.Cluster.MaintenancePolicy = &gkeapi.MaintenancePolicy{
			Window: &gkeapi.MaintenanceWindow{
				DailyMaintenanceWindow: &gkeapi.DailyMaintenanceWindow{
					StartTime: *config.Spec.MaintenanceWindow,
				},
			},
		}
	}

	// Autopilot Configuration
	if config.Spec.AutopilotConfig != nil && config.Spec.AutopilotConfig.Enabled {
		request.Cluster.Autopilot = &gkeapi.Autopilot{
			Enabled: config.Spec.AutopilotConfig.Enabled,
		}
	} else {
		// Configure addons for non-autopilot clusters
		addons := config.Spec.ClusterAddons
		request.Cluster.AddonsConfig = &gkeapi.AddonsConfig{
			HttpLoadBalancing:        &gkeapi.HttpLoadBalancing{Disabled: !addons.HTTPLoadBalancing},
			HorizontalPodAutoscaling: &gkeapi.HorizontalPodAutoscaling{Disabled: !addons.HorizontalPodAutoscaling},
			NetworkPolicyConfig:      &gkeapi.NetworkPolicyConfig{Disabled: !addons.NetworkPolicyConfig},
		}

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

	// Security Controls Implementation

	// Database Encryption (etcd encryption at rest with Cloud KMS)
	if config.Spec.DatabaseEncryption != nil {
		request.Cluster.DatabaseEncryption = &gkeapi.DatabaseEncryption{
			State:   config.Spec.DatabaseEncryption.State,
			KeyName: config.Spec.DatabaseEncryption.KeyName,
		}
	}

	// Binary Authorization
	if config.Spec.BinaryAuthorization != nil {
		request.Cluster.BinaryAuthorization = &gkeapi.BinaryAuthorization{
			Enabled: config.Spec.BinaryAuthorization.Enabled,
		}
	}

	// Shielded Nodes
	if config.Spec.ShieldedNodes != nil {
		request.Cluster.ShieldedNodes = &gkeapi.ShieldedNodes{
			Enabled: config.Spec.ShieldedNodes.Enabled,
		}
	}

	// Workload Identity
	if config.Spec.WorkloadIdentityConfig != nil {
		request.Cluster.WorkloadIdentityConfig = &gkeapi.WorkloadIdentityConfig{
			WorkloadPool: config.Spec.WorkloadIdentityConfig.WorkloadPool,
		}
	}

	// Legacy ABAC (should be disabled for security)
	if config.Spec.LegacyAbac != nil {
		request.Cluster.LegacyAbac = &gkeapi.LegacyAbac{
			Enabled: config.Spec.LegacyAbac.Enabled,
		}
	}

	// Master Authentication (basic auth and client certs should be disabled)
	if config.Spec.MasterAuth != nil {
		masterAuth := &gkeapi.MasterAuth{
			Username: config.Spec.MasterAuth.Username,
			Password: config.Spec.MasterAuth.Password,
		}
		if config.Spec.MasterAuth.ClientCertificateConfig != nil {
			masterAuth.ClientCertificateConfig = &gkeapi.ClientCertificateConfig{
				IssueClientCertificate: config.Spec.MasterAuth.ClientCertificateConfig.IssueClientCertificate,
			}
		}
		request.Cluster.MasterAuth = masterAuth
	}

	// Intra-node Visibility
	if config.Spec.IntraNodeVisibilityConfig != nil {
		request.Cluster.NetworkConfig = &gkeapi.NetworkConfig{
			EnableIntraNodeVisibility: config.Spec.IntraNodeVisibilityConfig.Enabled,
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
			return fmt.Errorf(cannotBeNilError, "nodePool.name", config.Spec.ClusterName, config.Name)
		}
		if nodeP[*np.Name] {
			return fmt.Errorf("nodePool name [%s] is not unique within the cluster [%s (id: %s)]", utils.StringValue(np.Name), config.Spec.ClusterName, config.Name)
		}
		nodeP[*np.Name] = true

		if np.Autoscaling != nil && np.Autoscaling.Enabled {
			if np.Autoscaling.MinNodeCount < 1 || np.Autoscaling.MaxNodeCount < np.Autoscaling.MinNodeCount {
				return fmt.Errorf("minNodeCount in the NodePool [%s] must be >= 1 and <= maxNodeCount within the cluster [%s (id: %s)]", utils.StringValue(np.Name), config.Spec.ClusterName, config.Name)
			}
		}
	}

	if config.Spec.CustomerManagedEncryptionKey != nil {
		if config.Spec.CustomerManagedEncryptionKey.RingName == "" ||
			config.Spec.CustomerManagedEncryptionKey.KeyName == "" {
			return fmt.Errorf("ringName and keyName are required to enable boot disk encryption for Node Pools within the cluster [%s (id: %s)]", config.Spec.ClusterName, config.Name)
		}
	}

	operation, err := gkeClient.ClusterList(
		ctx, LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone)))
	if err != nil {
		return err
	}

	for _, cluster := range operation.Clusters {
		if cluster.Name == config.Spec.ClusterName {
			return fmt.Errorf("cannot create cluster [%s (id: %s)] because a cluster in GKE exists with the same name, please delete and recreate with a different name", config.Spec.ClusterName, config.Name)
		}
	}

	if config.Spec.Imported {
		// Validation from here on out is for nilable attributes, not required for imported clusters
		return nil
	}

	if config.Spec.EnableKubernetesAlpha == nil {
		return fmt.Errorf(cannotBeNilError, "enableKubernetesAlpha", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.KubernetesVersion == nil {
		return fmt.Errorf(cannotBeNilError, "kubernetesVersion", config.Spec.ClusterName, config.Name)
	}
	// clusterIpv4CidrBlock is required when not using secondary ranges
	if config.Spec.ClusterIpv4CidrBlock == nil && config.Spec.IPAllocationPolicy != nil && config.Spec.IPAllocationPolicy.ClusterSecondaryRangeName == "" {
		return fmt.Errorf(cannotBeNilError, "clusterIpv4CidrBlock", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.ClusterAddons == nil {
		return fmt.Errorf(cannotBeNilError, "clusterAddons", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.IPAllocationPolicy == nil {
		return fmt.Errorf(cannotBeNilError, "ipAllocationPolicy", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.LoggingService == nil {
		return fmt.Errorf(cannotBeNilError, "loggingService", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.Network == nil {
		return fmt.Errorf(cannotBeNilError, "network", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.Subnetwork == nil {
		return fmt.Errorf(cannotBeNilError, "subnetwork", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.NetworkPolicyEnabled == nil {
		return fmt.Errorf(cannotBeNilError, "networkPolicyEnabled", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.PrivateClusterConfig == nil {
		return fmt.Errorf(cannotBeNilError, "privateClusterConfig", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.PrivateClusterConfig.EnablePrivateEndpoint && !config.Spec.PrivateClusterConfig.EnablePrivateNodes {
		return fmt.Errorf("private endpoint requires private nodes for cluster [%s (id: %s)]", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.MasterAuthorizedNetworksConfig == nil {
		return fmt.Errorf(cannotBeNilError, "masterAuthorizedNetworksConfig", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.MonitoringService == nil {
		return fmt.Errorf(cannotBeNilError, "monitoringService", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.Locations == nil {
		return fmt.Errorf(cannotBeNilError, "locations", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.MaintenanceWindow == nil {
		return fmt.Errorf(cannotBeNilError, "maintenanceWindow", config.Spec.ClusterName, config.Name)
	}
	if config.Spec.Labels == nil {
		return fmt.Errorf(cannotBeNilError, "labels", config.Spec.ClusterName, config.Name)
	}

	// Validate IP allocation policy to prevent CIDR conflicts
	// Validate cluster IP allocation
	if config.Spec.IPAllocationPolicy.ClusterIpv4CidrBlock != "" && config.Spec.IPAllocationPolicy.ClusterSecondaryRangeName != "" {
		return fmt.Errorf("cluster IP allocation conflict: cannot specify both clusterIpv4CidrBlock and clusterSecondaryRangeName for cluster [%s (id: %s)]. Use either CIDR block or secondary range name, not both", config.Spec.ClusterName, config.Name)
	}

	// Validate services IP allocation
	if config.Spec.IPAllocationPolicy.ServicesIpv4CidrBlock != "" && config.Spec.IPAllocationPolicy.ServicesSecondaryRangeName != "" {
		return fmt.Errorf("services IP allocation conflict: cannot specify both servicesIpv4CidrBlock and servicesSecondaryRangeName for cluster [%s (id: %s)]. Use either CIDR block or secondary range name, not both", config.Spec.ClusterName, config.Name)
	}

	// When using secondary ranges, ensure useIpAliases is enabled
	if (config.Spec.IPAllocationPolicy.ClusterSecondaryRangeName != "" || config.Spec.IPAllocationPolicy.ServicesSecondaryRangeName != "") && !config.Spec.IPAllocationPolicy.UseIPAliases {
		return fmt.Errorf("IP aliases must be enabled when using secondary ranges for cluster [%s (id: %s)]", config.Spec.ClusterName, config.Name)
	}

	// Validate that top-level clusterIpv4Cidr is not used with secondary ranges
	if config.Spec.ClusterIpv4CidrBlock != nil && config.Spec.IPAllocationPolicy.ClusterSecondaryRangeName != "" {
		return fmt.Errorf("cluster CIDR conflict: cannot specify both top-level clusterIpv4Cidr and ipAllocationPolicy.clusterSecondaryRangeName for cluster [%s (id: %s)]. When using secondary ranges, omit the top-level clusterIpv4Cidr field", config.Spec.ClusterName, config.Name)
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
		return fmt.Errorf(clusterErr, "nodePool.name", clusterName, config.Name)
	}
	if np.Version == nil {
		return fmt.Errorf(nodePoolErr, "version", *np.Name, clusterName, config.Name)
	}
	if np.Autoscaling == nil {
		return fmt.Errorf(nodePoolErr, "autoscaling", *np.Name, clusterName, config.Name)
	}
	if np.InitialNodeCount == nil {
		return fmt.Errorf(nodePoolErr, "initialNodeCount", *np.Name, clusterName, config.Name)
	}
	if np.MaxPodsConstraint == nil && config.Spec.IPAllocationPolicy != nil && config.Spec.IPAllocationPolicy.UseIPAliases {
		return fmt.Errorf(nodePoolErr, "maxPodsConstraint", *np.Name, clusterName, config.Name)
	}
	if np.Config == nil {
		return fmt.Errorf(nodePoolErr, "config", *np.Name, clusterName, config.Name)
	}
	if np.Management == nil {
		return fmt.Errorf(nodePoolErr, "management", *np.Name, clusterName, config.Name)
	}

	rxEmail := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-z]{2,}$`)
	if np.Config.ServiceAccount != "" && np.Config.ServiceAccount != "default" && !rxEmail.MatchString(np.Config.ServiceAccount) {
		return fmt.Errorf("field [%s] must either be an empty string, 'default' or set to a valid email address for nodepool [%s] in non-nil cluster [%s (id: %s)]", "serviceAccount", *np.Name, clusterName, config.Name)
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
			Enabled: np.Autoscaling.Enabled,
		},
		InitialNodeCount: *np.InitialNodeCount,
		Config: &gkeapi.NodeConfig{
			DiskSizeGb:     np.Config.DiskSizeGb,
			DiskType:       np.Config.DiskType,
			ImageType:      np.Config.ImageType,
			Labels:         np.Config.Labels,
			LocalSsdCount:  np.Config.LocalSsdCount,
			MachineType:    np.Config.MachineType,
			OauthScopes:    np.Config.OauthScopes,
			Preemptible:    np.Config.Preemptible,
			Tags:           np.Config.Tags,
			Taints:         taints,
			ServiceAccount: np.Config.ServiceAccount,
		},
		Version: *np.Version,
		Management: &gkeapi.NodeManagement{
			AutoRepair:  np.Management.AutoRepair,
			AutoUpgrade: np.Management.AutoUpgrade,
		},
	}

	// Only set autoscaling node counts when autoscaling is enabled
	if np.Autoscaling.Enabled {
		ret.Autoscaling.MaxNodeCount = np.Autoscaling.MaxNodeCount
		ret.Autoscaling.MinNodeCount = np.Autoscaling.MinNodeCount
	}

	if config.Spec.CustomerManagedEncryptionKey != nil &&
		config.Spec.CustomerManagedEncryptionKey.RingName != "" &&
		config.Spec.CustomerManagedEncryptionKey.KeyName != "" {
		// Determine which project ID to use for the KMS key
		keyProjectID := config.Spec.ProjectID
		if config.Spec.CustomerManagedEncryptionKey.ProjectID != "" {
			keyProjectID = config.Spec.CustomerManagedEncryptionKey.ProjectID
		}
		
		// Determine which region/zone to use for the KMS key location
		keyRegion := config.Spec.Region
		keyZone := config.Spec.Zone
		if config.Spec.CustomerManagedEncryptionKey.Region != "" {
			keyRegion = config.Spec.CustomerManagedEncryptionKey.Region
		}
		if config.Spec.CustomerManagedEncryptionKey.Zone != "" {
			keyZone = config.Spec.CustomerManagedEncryptionKey.Zone
		}
		keyLocation := Location(keyRegion, keyZone)
		
		ret.Config.BootDiskKmsKey = BootDiskRRN(
			keyProjectID,
			keyLocation,
			config.Spec.CustomerManagedEncryptionKey.RingName,
			config.Spec.CustomerManagedEncryptionKey.KeyName,
		)
	}

	// Check for node-pool-specific boot disk KMS key
	if np.Config.BootDiskKmsKey != "" {
		ret.Config.BootDiskKmsKey = np.Config.BootDiskKmsKey
	}

	// Security Controls for Node Pools

	// Shielded Instance Configuration (Integrity Monitoring and Secure Boot)
	if np.Config.ShieldedInstanceConfig != nil {
		ret.Config.ShieldedInstanceConfig = &gkeapi.ShieldedInstanceConfig{
			EnableIntegrityMonitoring: np.Config.ShieldedInstanceConfig.EnableIntegrityMonitoring,
			EnableSecureBoot:          np.Config.ShieldedInstanceConfig.EnableSecureBoot,
		}
	}

	// Workload Metadata Configuration (for GKE Metadata Server)
	if np.Config.WorkloadMetadataConfig != nil {
		ret.Config.WorkloadMetadataConfig = &gkeapi.WorkloadMetadataConfig{
			Mode: np.Config.WorkloadMetadataConfig.Mode,
		}
	}

	if config.Spec.IPAllocationPolicy.UseIPAliases {
		ret.MaxPodsConstraint = &gkeapi.MaxPodsConstraint{
			MaxPodsPerNode: *np.MaxPodsConstraint,
		}
	}
	return ret
}
