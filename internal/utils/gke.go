package utils

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	gkev1 "github.com/rancher/gke-operator/pkg/apis/gke.cattle.io/v1"
	wranglerv1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gkeapi "google.golang.org/api/container/v1"
	"google.golang.org/api/option"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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

// Network Providers
const (
	// NetworkProviderUnspecified is an unspecified netowrk provider
	NetworkProviderUnspecified = "PROVIDER_UNSPECIFIED"

	// NetworkProviderCalico describes the calico provider
	NetworkProviderCalico = "CALICO"
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

// GenerateGkeClusterCreateRequest creates a request
func GenerateGkeClusterCreateRequest(config *gkev1.GKEClusterConfig) (*gkeapi.CreateClusterRequest, error) {

	err := ValidateCreateRequest(config)
	if err != nil {
		return nil, err
	}

	request := &gkeapi.CreateClusterRequest{
		Cluster: &gkeapi.Cluster{
			NodePools: []*gkeapi.NodePool{},
		},
	}

	spec := config.Spec

	enableAlphaFeatures := config.Spec.EnableAlphaFeature != nil && !*config.Spec.EnableAlphaFeature

	request.Cluster.Name = config.Spec.ClusterName
	request.Cluster.InitialClusterVersion = *config.Spec.KubernetesVersion
	request.Cluster.Description = config.Spec.Description
	request.Cluster.EnableKubernetesAlpha = enableAlphaFeatures
	request.Cluster.IpAllocationPolicy = &gkeapi.IPAllocationPolicy{
		NodeIpv4CidrBlock:          spec.IPAllocationPolicy.ClusterIpv4CidrBlock,
		ClusterSecondaryRangeName:  spec.IPAllocationPolicy.ClusterSecondaryRangeName,
		CreateSubnetwork:           spec.IPAllocationPolicy.CreateSubnetwork,
		ServicesIpv4CidrBlock:      spec.IPAllocationPolicy.ServicesIpv4CidrBlock,
		ServicesSecondaryRangeName: spec.IPAllocationPolicy.ServicesSecondaryRangeName,
		SubnetworkName:             spec.IPAllocationPolicy.SubnetworkName,
		UseIpAliases:               spec.IPAllocationPolicy.UseIPAliases,
	}

	disableHTTPLoadBalancing := config.Spec.ClusterAddons.HTTPLoadBalancing != nil && !*config.Spec.ClusterAddons.HTTPLoadBalancing
	disableHorizontalPodAutoscaling := config.Spec.ClusterAddons.HorizontalPodAutoscaling != nil && !*config.Spec.ClusterAddons.HorizontalPodAutoscaling
	disableNetworkPolicyConfig := config.Spec.ClusterAddons.NetworkPolicyConfig != nil && !*config.Spec.ClusterAddons.NetworkPolicyConfig

	request.Cluster.AddonsConfig = &gkeapi.AddonsConfig{
		HttpLoadBalancing:        &gkeapi.HttpLoadBalancing{Disabled: !disableHTTPLoadBalancing},
		HorizontalPodAutoscaling: &gkeapi.HorizontalPodAutoscaling{Disabled: disableHorizontalPodAutoscaling},
		NetworkPolicyConfig:      &gkeapi.NetworkPolicyConfig{Disabled: disableNetworkPolicyConfig},
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

		asEnabled := np.Autoscaling.Enabled != nil && !*np.Autoscaling.Enabled

		as := &gkeapi.NodePoolAutoscaling{}
		if asEnabled {
			as.Enabled = asEnabled

			if np.Autoscaling.MaxNodeCount == nil {
				return nil, fmt.Errorf("max node count cant be nill")
			}
			as.MaxNodeCount = *np.Autoscaling.MaxNodeCount

			if np.Autoscaling.MinNodeCount == nil {
				return nil, fmt.Errorf("min node count cant be nil")
			}
			as.MinNodeCount = *np.Autoscaling.MinNodeCount
		}

		nodePool := &gkeapi.NodePool{
			Name:             *np.Name,
			Autoscaling:      as,
			InitialNodeCount: *np.InitialNodeCount,
			Config: &gkeapi.NodeConfig{
				DiskSizeGb:  *np.Config.DiskSizeGb,
				DiskType:    *np.Config.DiskType,
				ImageType:   *np.Config.ImageType,
				Labels:      np.Config.Labels,
				MachineType: *np.Config.MachineType,
				Taints:      taints,
				Preemptible: *np.Config.Preemptible,
			},
		}
		// If nill, use default
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

	if config.Spec.NetworkConfig != nil {
		request.Cluster.NetworkConfig = &gkeapi.NetworkConfig{
			Subnetwork: *config.Spec.NetworkConfig.Subnetwork,
			Network:    *config.Spec.NetworkConfig.Network,
		}
	}

	if config.Spec.NetworkPolicy != nil {
		request.Cluster.NetworkPolicy = &gkeapi.NetworkPolicy{
			Enabled:  *config.Spec.NetworkPolicy.Enabled,
			Provider: *config.Spec.NetworkPolicy.Provider,
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
func WaitClusterRemoveExp(ctx context.Context, svc *gkeapi.Service, config *gkev1.GKEClusterConfig) (*gkeapi.Operation, error) {
	var operation *gkeapi.Operation
	var err error

	for i := 1; i < 12; i++ {
		time.Sleep(time.Duration(i*i) * time.Second)
		operation, err = svc.Projects.Locations.Clusters.Delete(ClusterRRN(config.Spec.ProjectID, config.Spec.Region, config.Spec.ClusterName)).Context(ctx).Do()
		if err == nil {
			return operation, nil
		} else if !strings.Contains(err.Error(), "Please wait and try again once it is done") {
			break
		}
	}
	return operation, err
}

// GetClientset returns the clientset for a cluster
func GetClientset(cluster *gkeapi.Cluster, ts oauth2.TokenSource) (kubernetes.Interface, error) {
	capem, err := base64.StdEncoding.DecodeString(cluster.MasterAuth.ClusterCaCertificate)
	if err != nil {
		return nil, err
	}
	host := cluster.Endpoint
	if !strings.HasPrefix(host, "https://") {
		host = fmt.Sprintf("https://%s", host)
	}

	config := &rest.Config{
		Host: host,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: capem,
		},
		WrapTransport: func(rt http.RoundTripper) http.RoundTripper {
			return &oauth2.Transport{
				Source: ts,
				Base:   rt,
			}
		},
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return clientset, nil
}

// GetGKEClient returns a gke client capable of making requests on behalf of the services account
func GetGKEClient(ctx context.Context, secretsCache wranglerv1.SecretCache, config *gkev1.GKEClusterConfig) (*gkeapi.Service, error) {
	ns, id := Parse(config.Spec.CredentialContent)
	secret, err := secretsCache.Get(ns, id)

	if err != nil {
		return nil, err
	}

	dataBytes := secret.Data["gkeCredentialConfig-data"]

	data := string(dataBytes)

	return GetServiceClient(ctx, data)
}

// ValidateCreateRequest checks a config for the ability to generate a create request
func ValidateCreateRequest(config *gkev1.GKEClusterConfig) error {
	//TODO: check if these can even be nill/empty in a inported cluster.
	if config.Spec.ProjectID == "" {
		return fmt.Errorf("project ID is required")
	} else if config.Spec.Zone == "" && config.Spec.Region == "" {
		return fmt.Errorf("zone or region is required")
	} else if config.Spec.Zone != "" && config.Spec.Region != "" {
		return fmt.Errorf("only one of zone or region must be specified")
	} else if config.Spec.ClusterName == "" {
		return fmt.Errorf("cluster name is required")
	}

	for _, np := range config.Spec.NodePools {
		if np.Autoscaling.Enabled != nil && *np.Autoscaling.Enabled {
			if np.Autoscaling.MaxNodeCount == nil || np.Autoscaling.MinNodeCount == nil {
				return fmt.Errorf("min and maxNodeCount can't be nil")
			}
			if *np.Autoscaling.MinNodeCount < 1 || *np.Autoscaling.MaxNodeCount < *np.Autoscaling.MinNodeCount {
				return fmt.Errorf("minNodeCount in the NodePool must be >= 1 and <= maxNodeCount")
			}
		}
	}

	if config.Spec.IPAllocationPolicy == nil {
		return fmt.Errorf("IpAllocationPolicy can't be nil")
	}

	//check if cluster with same name exists
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc, err := GetServiceClient(ctx, config.Spec.CredentialContent)
	if err != nil {
		return err
	}
	operation, err := svc.Projects.Locations.Clusters.List(
		LocationRRN(config.Spec.ProjectID, config.Spec.Region)).Context(ctx).Do()

	for _, cluster := range operation.Clusters {
		if cluster.Name == config.Spec.ClusterName {
			return fmt.Errorf("cannot create cluster [%s] because a cluster in GKE exists with the same name", config.Spec.ClusterName)
		}
	}

	/*
		if !config.Spec.Imported {
			cannotBeNilError := "field [%s] cannot be nil for non-import cluster [%s]"
			if config.Spec.KubernetesVersion == nil {
				return fmt.Errorf(cannotBeNilError, "kubernetesVersion", config.Name)
			}
			if config.Spec.LoggingTypes == nil {
				return fmt.Errorf(cannotBeNilError, "loggingTypes", config.Name)
			}
		}
	*/
	for _, np := range config.Spec.NodePools {
		cannotBeNilError := "field [%s] cannot be nil for nodegroup [%s] in non-nil cluster [%s]"
		if !config.Spec.Imported {
			if np.Name == nil {
				return fmt.Errorf(cannotBeNilError, "name", np.Name, config.Name)
			}
			if np.Version == nil {
				return fmt.Errorf(cannotBeNilError, "version", np.Name, config.Name)
			}
			if np.Autoscaling.MinNodeCount == nil {
				return fmt.Errorf(cannotBeNilError, "minNodeCount", np.Name, config.Name)
			}
			if np.Autoscaling.MaxNodeCount == nil {
				return fmt.Errorf(cannotBeNilError, "maxNodeCount", np.Name, config.Name)
			}
			if np.InitialNodeCount == nil {
				return fmt.Errorf(cannotBeNilError, "initialNodeCount", np.Name, config.Name)
			}
			if np.Config.DiskSizeGb == nil {
				return fmt.Errorf(cannotBeNilError, "diskSizeGb", np.Name, config.Name)
			}
			if np.Config.DiskType == nil {
				return fmt.Errorf(cannotBeNilError, "diskType", np.Name, config.Name)
			}
			if np.Config.ImageType == nil {
				return fmt.Errorf(cannotBeNilError, "imageType", np.Name, config.Name)
			}
			if np.Config.MachineType == nil {
				return fmt.Errorf(cannotBeNilError, "machineType", np.Name, config.Name)
			}
			if np.Config.Preemptible == nil {
				return fmt.Errorf(cannotBeNilError, "preemptible", np.Name, config.Name)
			}
		}
	}

	return nil
}

func GetServiceClient(ctx context.Context, credential string) (*gkeapi.Service, error) {
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

func UpdateCluster(config *gkev1.GKEClusterConfig, updateRequest *gkeapi.UpdateClusterRequest) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc, err := GetServiceClient(ctx, config.Spec.CredentialContent)
	if err != nil {
		return err
	}

	_, err = svc.Projects.Locations.Clusters.Update(ClusterRRN(config.Spec.ProjectID, config.Spec.Region, config.Spec.ClusterName), updateRequest).Context(ctx).Do()

	return err
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
	newSpec := &gkev1.GKEClusterConfigSpec{}

	newSpec.KubernetesVersion = &upstreamSpec.CurrentMasterVersion
	newSpec.EnableAlphaFeature = &upstreamSpec.EnableKubernetesAlpha

	// build node groups
	newSpec.NodePools = make([]gkev1.NodePoolConfig, 0, len(upstreamSpec.NodePools))

	for _, np := range upstreamSpec.NodePools {
		if np.Status == NodePoolStatusStopping {
			continue
		}

		newNP := gkev1.NodePoolConfig{
			Name: &np.Name,
		}

		if np.Config != nil {
			newNP.Config = &gkev1.NodeConfig{
				DiskSizeGb:    &np.Config.DiskSizeGb,
				DiskType:      &np.Config.DiskType,
				ImageType:     &np.Config.ImageType,
				Labels:        np.Config.Labels,
				LocalSsdCount: &np.Config.LocalSsdCount,
				MachineType:   &np.Config.MachineType,
				Preemptible:   &np.Config.Preemptible,
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
				Enabled:      &np.Autoscaling.Enabled,
				MaxNodeCount: &np.Autoscaling.MaxNodeCount,
				MinNodeCount: &np.Autoscaling.MinNodeCount,
			}
		}

		newSpec.NodePools = append(newSpec.NodePools, newNP)
	}

	return newSpec, nil
}
