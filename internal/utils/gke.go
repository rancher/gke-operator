package utils

import (
	"context"
	"fmt"
	"strings"
	"time"

	gkev1 "github.com/rancher/gke-operator/pkg/apis/gke.cattle.io/v1"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gkeapi "google.golang.org/api/container/v1"
	"google.golang.org/api/option"
)

// Node Pool Status
const (
	// NodePoolStatusUnspecified - Not set.
	NodePoolStatusUnspecified = "STATUS_UNSPECIFIED"
	// NodePoolStatusProvisioning The PROVISIONING state indicates the node pool is
	// being created.
	NodePoolStatusProvisioning = "PROVISIONING"
	// NodePoolStatusRunning The RUNNING state indicates the node pool has been
	// created
	// and is fully usable.
	NodePoolStatusRunning = "RUNNING"
	// NodePoolStatusRunningWithError The RUNNING_WITH_ERROR state indicates the
	// node pool has been created
	// and is partially usable. Some error state has occurred and
	// some
	// functionality may be impaired. Customer may need to reissue a
	// request
	// or trigger a new update.
	NodePoolStatusRunningWithError = "RUNNING_WITH_ERROR"
	// NodePoolStatusReconciling The RECONCILING state indicates that some work is
	// actively being done on
	// the node pool, such as upgrading node software. Details can
	// be found in the `statusMessage` field.
	NodePoolStatusReconciling = "RECONCILING"
	//  NodePoolStatusStopping The STOPPING state indicates the node pool is being
	// deleted.
	NodePoolStatusStopping = "STOPPING"
	// NodePoolStatusError The ERROR state indicates the node pool may be unusable.
	// Details
	NodePoolStatusError = "ERROR"
)

// Cluster Status
const (
	// ClusterStatusProvisioning The PROVISIONING state indicates the cluster is
	// being created.
	ClusterStatusProvisioning = "PROVISIONING"

	// ClusterStatusRunning The RUNNING state indicates the cluster has been
	// created and is fully
	ClusterStatusRunning = "RUNNING"

	// ClusterStatusStopping The STOPPING state indicates the cluster is being
	// deleted.
	ClusterStatusStopping = "STOPPING"

	// ClusterStatusError is a ClusterStatus enum value
	ClusterStatusError = "ERROR"

	// ClusterStatusReconciling The RECONCILING state indicates that some work is
	// actively being done on
	ClusterStatusReconciling = "RECONCILING"

	// ClusterStatusUnspecified - Not set.
	ClusterStatusUnspecified = "STATUS_UNSPECIFIED"

	// ClusterStatusDegraded The DEGRADED state indicates the cluster requires user
	// action to restore
	ClusterStatusDegraded = "DEGRADED"
)

// Errors
const (
	cannotBeNilError            = "field [%s] cannot be nil for non-import cluster [%s]"
	cannotBeNilForNodePoolError = "field [%s] cannot be nil for nodepool [%s] in non-nil cluster [%s]"
)

// GenerateGkeClusterCreateRequest creates a request
func GenerateGkeClusterCreateRequest(client *gkeapi.Service, config *gkev1.GKEClusterConfig) (*gkeapi.CreateClusterRequest, error) {

	err := ValidateCreateRequest(client, config)
	if err != nil {
		return nil, err
	}

	enableAlphaFeatures := config.Spec.EnableAlphaFeature != nil && *config.Spec.EnableAlphaFeature
	request := &gkeapi.CreateClusterRequest{
		Cluster: &gkeapi.Cluster{
			Name:                  config.Spec.ClusterName,
			Description:           config.Spec.Description,
			InitialClusterVersion: *config.Spec.KubernetesVersion,
			EnableKubernetesAlpha: enableAlphaFeatures,
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
			AddonsConfig: &gkeapi.AddonsConfig{},
			NodePools:    []*gkeapi.NodePool{},
		},
	}

	addons := config.Spec.ClusterAddons
	if addons != nil {
		request.Cluster.AddonsConfig.HttpLoadBalancing = &gkeapi.HttpLoadBalancing{Disabled: !addons.HTTPLoadBalancing}
		request.Cluster.AddonsConfig.HorizontalPodAutoscaling = &gkeapi.HorizontalPodAutoscaling{Disabled: !addons.HorizontalPodAutoscaling}
		request.Cluster.AddonsConfig.NetworkPolicyConfig = &gkeapi.NetworkPolicyConfig{Disabled: !addons.NetworkPolicyConfig}
	}

	request.Cluster.NodePools = make([]*gkeapi.NodePool, 0, len(config.Spec.NodePools))

	for _, np := range config.Spec.NodePools {

		var taints []*gkeapi.NodeTaint = make([]*gkeapi.NodeTaint, 0, len(np.Config.Taints))
		for _, t := range np.Config.Taints {
			taints = append(taints, &gkeapi.NodeTaint{
				Effect: t.Effect,
				Key:    t.Key,
				Value:  t.Value,
			})
		}

		nodePool := &gkeapi.NodePool{
			Name: *np.Name,
			Autoscaling: &gkeapi.NodePoolAutoscaling{
				Enabled:      np.Autoscaling.Enabled,
				MaxNodeCount: np.Autoscaling.MaxNodeCount,
				MinNodeCount: np.Autoscaling.MinNodeCount,
			},
			InitialNodeCount: *np.InitialNodeCount,
			Config: &gkeapi.NodeConfig{
				DiskSizeGb:  np.Config.DiskSizeGb,
				DiskType:    np.Config.DiskType,
				ImageType:   np.Config.ImageType,
				Labels:      np.Config.Labels,
				MachineType: np.Config.MachineType,
				OauthScopes: np.Config.OauthScopes,
				Taints:      taints,
				Preemptible: np.Config.Preemptible,
			},
		}
		// If nil, use default
		if np.MaxPodsConstraint != nil {
			nodePool.MaxPodsConstraint = &gkeapi.MaxPodsConstraint{
				MaxPodsPerNode: *np.MaxPodsConstraint,
			}
		}

		request.Cluster.NodePools = append(request.Cluster.NodePools, nodePool)
	}

	if config.Spec.MasterAuthorizedNetworksConfig != nil {
		var blocks = make([]*gkeapi.CidrBlock, len(config.Spec.MasterAuthorizedNetworksConfig.CidrBlocks))
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

	if config.Spec.GKEClusterNetworkConfig != nil {
		request.Cluster.NetworkConfig = &gkeapi.NetworkConfig{
			Subnetwork: *config.Spec.GKEClusterNetworkConfig.Subnetwork,
			Network:    *config.Spec.GKEClusterNetworkConfig.Network,
		}
	}

	if config.Spec.NetworkPolicyEnabled != nil {
		request.Cluster.NetworkPolicy = &gkeapi.NetworkPolicy{
			Enabled: *config.Spec.NetworkPolicyEnabled,
		}
	}

	if config.Spec.PrivateClusterConfig != nil {
		request.Cluster.PrivateClusterConfig = &gkeapi.PrivateClusterConfig{
			EnablePrivateEndpoint: *config.Spec.PrivateClusterConfig.EnablePrivateEndpoint,
			EnablePrivateNodes:    *config.Spec.PrivateClusterConfig.EnablePrivateNodes,
			MasterIpv4CidrBlock:   config.Spec.PrivateClusterConfig.MasterIpv4CidrBlock,
			PrivateEndpoint:       config.Spec.PrivateClusterConfig.PrivateEndpoint,
			PublicEndpoint:        config.Spec.PrivateClusterConfig.PublicEndpoint,
		}
	}

	return request, nil
}

// WaitClusterRemoveExp waits for a cluster to be removed
func WaitClusterRemoveExp(ctx context.Context, client *gkeapi.Service, config *gkev1.GKEClusterConfig) (*gkeapi.Operation, error) {
	var operation *gkeapi.Operation
	var err error

	for i := 1; i < 12; i++ {
		time.Sleep(time.Duration(i*i) * time.Second)
		operation, err = client.Projects.Locations.Clusters.Delete(ClusterRRN(config.Spec.ProjectID, config.Spec.Region, config.Spec.ClusterName)).Context(ctx).Do()
		if err == nil {
			return operation, nil
		} else if !strings.Contains(err.Error(), "Please wait and try again once it is done") {
			break
		}
	}
	return operation, err
}

// ValidateCreateRequest checks a config for the ability to generate a create request
func ValidateCreateRequest(client *gkeapi.Service, config *gkev1.GKEClusterConfig) error {
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

	//check if cluster with same name exists
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	operation, err := client.Projects.Locations.Clusters.List(
		LocationRRN(config.Spec.ProjectID, config.Spec.Region)).Context(ctx).Do()
	if err != nil {
		return err
	}

	for _, cluster := range operation.Clusters {
		if cluster.Name == config.Spec.ClusterName {
			return fmt.Errorf("cannot create cluster [%s] because a cluster in GKE exists with the same name", config.Spec.ClusterName)
		}
	}

	if config.Spec.Imported {
		// Validation from here on out is for nilable attributes, not required for imported clusters
		return nil
	}

	if config.Spec.EnableAlphaFeature == nil {
		return fmt.Errorf(cannotBeNilError, "enableAlphaFeature", config.Name)
	}
	if config.Spec.KubernetesVersion == nil {
		return fmt.Errorf(cannotBeNilError, "kubernetesVersion", config.Name)
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
	if config.Spec.GKEClusterNetworkConfig == nil {
		return fmt.Errorf(cannotBeNilError, "gkeClusterNetworkConfig", config.Name)
	}
	if config.Spec.NetworkPolicyEnabled == nil {
		return fmt.Errorf(cannotBeNilError, "networkPolicyEnabled", config.Name)
	}
	if config.Spec.PrivateClusterConfig == nil {
		return fmt.Errorf(cannotBeNilError, "privateClusterConfig", config.Name)
	}
	if config.Spec.MasterAuthorizedNetworksConfig == nil {
		return fmt.Errorf(cannotBeNilError, "masterAuthorizedNetworksConfig", config.Name)
	}
	if config.Spec.MonitoringService == nil {
		return fmt.Errorf(cannotBeNilError, "monitoringService", config.Name)
	}

	for _, np := range config.Spec.NodePools {
		if np.Name == nil {
			return fmt.Errorf(cannotBeNilError, "nodePool.name", config.Name)
		}
		cannotBeNil := cannotBeNilForNodePoolError
		if np.Version == nil {
			return fmt.Errorf(cannotBeNil, "version", *np.Name, config.Name)
		}
		if np.Autoscaling == nil {
			return fmt.Errorf(cannotBeNil, "autoscaling", *np.Name, config.Name)
		}
		if np.InitialNodeCount == nil {
			return fmt.Errorf(cannotBeNil, "initialNodeCount", *np.Name, config.Name)
		}
		if np.MaxPodsConstraint == nil {
			return fmt.Errorf(cannotBeNil, "maxPodsConstraint", *np.Name, config.Name)
		}
		if np.Config == nil {
			return fmt.Errorf(cannotBeNil, "config", *np.Name, config.Name)
		}
	}

	return nil
}

func GetGKEClient(ctx context.Context, credential string) (*gkeapi.Service, error) {
	ts, err := GetTokenSource(ctx, credential)
	if err != nil {
		return nil, err
	}
	return GetServiceClientWithTokenSource(ctx, ts)
}

func GetServiceClientWithTokenSource(ctx context.Context, ts oauth2.TokenSource) (*gkeapi.Service, error) {
	return gkeapi.NewService(ctx, option.WithHTTPClient(oauth2.NewClient(ctx, ts)))
}

func GetTokenSource(ctx context.Context, credential string) (oauth2.TokenSource, error) {
	ts, err := google.CredentialsFromJSON(ctx, []byte(credential), gkeapi.CloudPlatformScope)
	if err != nil {
		return nil, err
	}
	return ts.TokenSource, nil
}

// LocationRRN returns a Relative Resource Name representing a location. This
// RRN can either represent a Region or a Zone. It can be used as the parent
// attribute during cluster creation to create a zonal or regional cluster, or
// be used to generate more specific RRNs like an RRN representing a cluster.
//
// https://cloud.google.com/apis/design/resource_names#relative_resource_name
func LocationRRN(projectID, location string) string {
	return fmt.Sprintf("projects/%s/locations/%s", projectID, location)
}

// ClusterRRN returns an Relative Resource Name of a cluster in the specified
// region or zone
func ClusterRRN(projectID, location, clusterName string) string {
	return fmt.Sprintf("%s/clusters/%s", LocationRRN(projectID, location), clusterName)
}

// NodePoolRRN returns a Relative Resource Name of a node pool in a cluster in the
// region or zone for the specified project
func NodePoolRRN(projectID, location, clusterName, nodePool string) string {
	return fmt.Sprintf("%s/nodePools/%s", ClusterRRN(projectID, location, clusterName), nodePool)
}

func BuildUpstreamClusterState(upstreamSpec *gkeapi.Cluster) (*gkev1.GKEClusterConfigSpec, error) {
	newSpec := &gkev1.GKEClusterConfigSpec{
		KubernetesVersion:       &upstreamSpec.CurrentMasterVersion,
		EnableAlphaFeature:      &upstreamSpec.EnableKubernetesAlpha,
		ClusterAddons:           &gkev1.ClusterAddons{},
		ClusterIpv4CidrBlock:    upstreamSpec.ClusterIpv4Cidr,
		LoggingService:          &upstreamSpec.LoggingService,
		MonitoringService:       &upstreamSpec.MonitoringService,
		GKEClusterNetworkConfig: &gkev1.GKEClusterNetworkConfig{},
		PrivateClusterConfig:    &gkev1.PrivateClusterConfig{},
		IPAllocationPolicy:      &gkev1.IPAllocationPolicy{},
		MasterAuthorizedNetworksConfig: &gkev1.MasterAuthorizedNetworksConfig{
			Enabled: false,
		},
	}

	networkPolicyEnabled := false
	if upstreamSpec.NetworkPolicy != nil && upstreamSpec.NetworkPolicy.Enabled == true {
		networkPolicyEnabled = true
	}
	newSpec.NetworkPolicyEnabled = &networkPolicyEnabled

	if upstreamSpec.NetworkConfig != nil {
		newSpec.GKEClusterNetworkConfig.Network = &upstreamSpec.NetworkConfig.Network
		newSpec.GKEClusterNetworkConfig.Subnetwork = &upstreamSpec.NetworkConfig.Subnetwork
	} else {
		network := "default"
		newSpec.GKEClusterNetworkConfig.Network = &network
		newSpec.GKEClusterNetworkConfig.Subnetwork = &network
	}

	if upstreamSpec.PrivateClusterConfig != nil {
		newSpec.PrivateClusterConfig.EnablePrivateEndpoint = &upstreamSpec.PrivateClusterConfig.EnablePrivateNodes
		newSpec.PrivateClusterConfig.EnablePrivateNodes = &upstreamSpec.PrivateClusterConfig.EnablePrivateNodes
		newSpec.PrivateClusterConfig.MasterIpv4CidrBlock = upstreamSpec.PrivateClusterConfig.MasterIpv4CidrBlock
		newSpec.PrivateClusterConfig.PrivateEndpoint = upstreamSpec.PrivateClusterConfig.PrivateEndpoint
		newSpec.PrivateClusterConfig.PublicEndpoint = upstreamSpec.PrivateClusterConfig.PublicEndpoint
	} else {
		enabled := false
		newSpec.PrivateClusterConfig.EnablePrivateEndpoint = &enabled
		newSpec.PrivateClusterConfig.EnablePrivateNodes = &enabled
	}

	// build cluster addons
	if upstreamSpec.AddonsConfig != nil {
		lb := true
		if upstreamSpec.AddonsConfig.HttpLoadBalancing != nil {
			lb = !upstreamSpec.AddonsConfig.HttpLoadBalancing.Disabled
		}
		newSpec.ClusterAddons.HTTPLoadBalancing = lb
		hpa := true
		if upstreamSpec.AddonsConfig.HorizontalPodAutoscaling != nil {
			hpa = !upstreamSpec.AddonsConfig.HorizontalPodAutoscaling.Disabled
		}
		newSpec.ClusterAddons.HorizontalPodAutoscaling = hpa
		npc := true
		if upstreamSpec.AddonsConfig.NetworkPolicyConfig != nil {
			npc = !upstreamSpec.AddonsConfig.NetworkPolicyConfig.Disabled
		}
		newSpec.ClusterAddons.NetworkPolicyConfig = npc
	}

	if upstreamSpec.IpAllocationPolicy != nil {
		newSpec.IPAllocationPolicy.ClusterIpv4CidrBlock = upstreamSpec.IpAllocationPolicy.ClusterIpv4CidrBlock
		newSpec.IPAllocationPolicy.ClusterSecondaryRangeName = upstreamSpec.IpAllocationPolicy.ClusterSecondaryRangeName
		newSpec.IPAllocationPolicy.CreateSubnetwork = upstreamSpec.IpAllocationPolicy.CreateSubnetwork
		newSpec.IPAllocationPolicy.NodeIpv4CidrBlock = upstreamSpec.IpAllocationPolicy.NodeIpv4CidrBlock
		newSpec.IPAllocationPolicy.ServicesIpv4CidrBlock = upstreamSpec.IpAllocationPolicy.ServicesIpv4CidrBlock
		newSpec.IPAllocationPolicy.ServicesSecondaryRangeName = upstreamSpec.IpAllocationPolicy.ServicesSecondaryRangeName
		newSpec.IPAllocationPolicy.SubnetworkName = upstreamSpec.IpAllocationPolicy.SubnetworkName
		newSpec.IPAllocationPolicy.UseIPAliases = upstreamSpec.IpAllocationPolicy.UseIpAliases
	}

	if upstreamSpec.MasterAuthorizedNetworksConfig != nil {
		if upstreamSpec.MasterAuthorizedNetworksConfig.Enabled {
			newSpec.MasterAuthorizedNetworksConfig.Enabled = upstreamSpec.MasterAuthorizedNetworksConfig.Enabled
			for _, b := range upstreamSpec.MasterAuthorizedNetworksConfig.CidrBlocks {
				block := &gkev1.CidrBlock{
					CidrBlock:   b.CidrBlock,
					DisplayName: b.DisplayName,
				}
				newSpec.MasterAuthorizedNetworksConfig.CidrBlocks = append(newSpec.MasterAuthorizedNetworksConfig.CidrBlocks, block)
			}
		}
	}

	// build node groups
	newSpec.NodePools = make([]gkev1.NodePoolConfig, 0, len(upstreamSpec.NodePools))

	for _, np := range upstreamSpec.NodePools {
		if np.Status == NodePoolStatusStopping {
			continue
		}

		newNP := gkev1.NodePoolConfig{
			Name:              &np.Name,
			Version:           &np.Version,
			InitialNodeCount:  &np.InitialNodeCount,
			MaxPodsConstraint: &np.MaxPodsConstraint.MaxPodsPerNode,
		}

		if np.Config != nil {
			newNP.Config = &gkev1.NodeConfig{
				DiskSizeGb:    np.Config.DiskSizeGb,
				DiskType:      np.Config.DiskType,
				ImageType:     np.Config.ImageType,
				Labels:        np.Config.Labels,
				LocalSsdCount: np.Config.LocalSsdCount,
				MachineType:   np.Config.MachineType,
				Preemptible:   np.Config.Preemptible,
			}

			newNP.Config.Taints = make([]gkev1.NodeTaintConfig, 0, len(np.Config.Taints))
			for _, t := range np.Config.Taints {
				newNP.Config.Taints = append(newNP.Config.Taints, gkev1.NodeTaintConfig{
					Effect: t.Effect,
					Key:    t.Key,
					Value:  t.Value,
				})
			}
		}

		if np.Autoscaling != nil {
			newNP.Autoscaling = &gkev1.NodePoolAutoscaling{
				Enabled:      np.Autoscaling.Enabled,
				MaxNodeCount: np.Autoscaling.MaxNodeCount,
				MinNodeCount: np.Autoscaling.MinNodeCount,
			}
		}

		newSpec.NodePools = append(newSpec.NodePools, newNP)
	}

	return newSpec, nil
}
