package gke

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/rancher/gke-operator/internal/utils"
	gkev1 "github.com/rancher/gke-operator/pkg/apis/gke.cattle.io/v1"
	"github.com/sirupsen/logrus"
	gkeapi "google.golang.org/api/container/v1"
)

// Network Providers
const (
	// NetworkProviderCalico describes the calico provider
	NetworkProviderCalico = "CALICO"
)

// Logging Services
const (
	// CloudLoggingService is the Cloud Logging service with a Kubernetes-native resource model
	CloudLoggingService = "logging.googleapis.com/kubernetes"
)

// Monitoring Services
const (
	// CloudMonitoringService is the Cloud Monitoring service with a Kubernetes-native resource model
	CloudMonitoringService = "monitoring.googleapis.com/kubernetes"
)

// UpdateMasterKubernetesVersion updates the Kubernetes version for the control plane.
// This must occur before the Kubernetes version is changed on the nodes.
func UpdateMasterKubernetesVersion(ctx context.Context, client *gkeapi.Service, config *gkev1.GKEClusterConfig, upstreamSpec *gkev1.GKEClusterConfigSpec) (Status, error) {
	kubeVersion := utils.StringValue(config.Spec.KubernetesVersion)
	if kubeVersion == "" {
		return NotChanged, nil
	}

	if utils.StringValue(upstreamSpec.KubernetesVersion) == kubeVersion {
		return NotChanged, nil
	}

	logrus.Infof("updating kubernetes version for cluster [%s]", config.Name)
	_, err := client.Projects.
		Locations.
		Clusters.
		Update(
			ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName),
			&gkeapi.UpdateClusterRequest{
				Update: &gkeapi.ClusterUpdate{
					DesiredMasterVersion: kubeVersion,
				},
			},
		).Context(ctx).
		Do()
	if err != nil {
		return NotChanged, err
	}
	return Changed, nil
}

// UpdateClusterAddons updates the cluster addons.
// In the case of the NetworkPolicyConfig addon, this may need to be retried after NetworkPolicyEnabled has been updated.
func UpdateClusterAddons(ctx context.Context, client *gkeapi.Service, config *gkev1.GKEClusterConfig, upstreamSpec *gkev1.GKEClusterConfigSpec) (Status, error) {
	clusterUpdate := &gkeapi.ClusterUpdate{}
	addons := config.Spec.ClusterAddons
	if addons == nil {
		return NotChanged, nil
	}

	needsUpdate := false
	if upstreamSpec.ClusterAddons.HTTPLoadBalancing != addons.HTTPLoadBalancing {
		clusterUpdate.DesiredAddonsConfig = &gkeapi.AddonsConfig{}
		clusterUpdate.DesiredAddonsConfig.HttpLoadBalancing = &gkeapi.HttpLoadBalancing{
			Disabled: !addons.HTTPLoadBalancing,
		}
		needsUpdate = true
	}
	if upstreamSpec.ClusterAddons.HorizontalPodAutoscaling != addons.HorizontalPodAutoscaling {
		if clusterUpdate.DesiredAddonsConfig == nil {
			clusterUpdate.DesiredAddonsConfig = &gkeapi.AddonsConfig{}
		}
		clusterUpdate.DesiredAddonsConfig.HorizontalPodAutoscaling = &gkeapi.HorizontalPodAutoscaling{
			Disabled: !addons.HorizontalPodAutoscaling,
		}
		needsUpdate = true
	}
	if upstreamSpec.ClusterAddons.NetworkPolicyConfig != addons.NetworkPolicyConfig {
		// If disabling NetworkPolicyConfig for the cluster, NetworkPolicyEnabled
		// (which affects nodes) needs to be disabled first. If
		// NetworkPolicyEnabled is already set to false downstream but not yet
		// complete upstream, that update will be enqueued later in this
		// sequence and we just need to wait for it to complete.
		if !addons.NetworkPolicyConfig && !*config.Spec.NetworkPolicyEnabled && *upstreamSpec.NetworkPolicyEnabled {
			logrus.Infof("waiting to update NetworkPolicyConfig cluster addon")
		} else {
			if clusterUpdate.DesiredAddonsConfig == nil {
				clusterUpdate.DesiredAddonsConfig = &gkeapi.AddonsConfig{}
			}
			clusterUpdate.DesiredAddonsConfig.NetworkPolicyConfig = &gkeapi.NetworkPolicyConfig{
				Disabled: !addons.NetworkPolicyConfig,
			}
			needsUpdate = true
		}
	}

	if needsUpdate {
		logrus.Infof("updating addon configuration for cluster [%s]", config.Name)
		_, err := client.Projects.
			Locations.
			Clusters.
			Update(
				ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName),
				&gkeapi.UpdateClusterRequest{
					Update: clusterUpdate,
				},
			).Context(ctx).
			Do()
		// In the case of disabling both NetworkPolicyEnabled and NetworkPolicyConfig,
		// the node pool will automatically be recreated after NetworkPolicyEnabled is
		// disabled, so we need to wait. Capture this event and log it as Info
		// rather than an error, since this is a normal but potentially
		// time-consuming event.
		if err != nil {
			matched, matchErr := regexp.MatchString(`Node pool "\S+" requires recreation`, err.Error())
			if matchErr != nil {
				return NotChanged, fmt.Errorf("programming error: %w", matchErr)
			}
			if matched {
				logrus.Infof("waiting for node pool to finish recreation")
				return Retry, nil
			}
			return NotChanged, err
		}
		return Changed, nil
	}
	return NotChanged, nil
}

// UpdateMasterAuthorizedNetworks updates MasterAuthorizedNetworks
func UpdateMasterAuthorizedNetworks(
	ctx context.Context,
	client *gkeapi.Service,
	config *gkev1.GKEClusterConfig,
	upstreamSpec *gkev1.GKEClusterConfigSpec) (Status, error) {
	if config.Spec.MasterAuthorizedNetworksConfig == nil {
		return NotChanged, nil
	}

	clusterUpdate := &gkeapi.ClusterUpdate{}
	needsUpdate := false
	if upstreamSpec.MasterAuthorizedNetworksConfig.Enabled != config.Spec.MasterAuthorizedNetworksConfig.Enabled {
		clusterUpdate.DesiredMasterAuthorizedNetworksConfig = &gkeapi.MasterAuthorizedNetworksConfig{
			Enabled: config.Spec.MasterAuthorizedNetworksConfig.Enabled,
		}
		needsUpdate = true
	}
	if config.Spec.MasterAuthorizedNetworksConfig.Enabled && !compareCidrBlockPointerSlices(
		upstreamSpec.MasterAuthorizedNetworksConfig.CidrBlocks,
		config.Spec.MasterAuthorizedNetworksConfig.CidrBlocks) {
		if clusterUpdate.DesiredMasterAuthorizedNetworksConfig == nil {
			clusterUpdate.DesiredMasterAuthorizedNetworksConfig = &gkeapi.MasterAuthorizedNetworksConfig{
				Enabled: true,
			}
		}
		for _, v := range config.Spec.MasterAuthorizedNetworksConfig.CidrBlocks {
			clusterUpdate.DesiredMasterAuthorizedNetworksConfig.CidrBlocks = append(
				clusterUpdate.DesiredMasterAuthorizedNetworksConfig.CidrBlocks,
				&gkeapi.CidrBlock{
					CidrBlock:   v.CidrBlock,
					DisplayName: v.DisplayName,
				})
		}
		needsUpdate = true
	}
	if needsUpdate {
		logrus.Infof("updating master authorized networks configuration for cluster [%s]", config.Name)
		_, err := client.Projects.
			Locations.
			Clusters.
			Update(
				ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName),
				&gkeapi.UpdateClusterRequest{
					Update: clusterUpdate,
				},
			).Context(ctx).
			Do()
		if err != nil {
			return NotChanged, err
		}
		return Changed, nil
	}
	return NotChanged, nil
}

// UpdateLoggingMonitoringService updates both LoggingService and MonitoringService.
// In most cases, updating one requires explicitly updating the other as well, so these are paired.
func UpdateLoggingMonitoringService(
	ctx context.Context,
	client *gkeapi.Service,
	config *gkev1.GKEClusterConfig,
	upstreamSpec *gkev1.GKEClusterConfigSpec) (Status, error) {
	clusterUpdate := &gkeapi.ClusterUpdate{}
	needsUpdate := false
	if config.Spec.LoggingService != nil {
		loggingService := *config.Spec.LoggingService
		if loggingService == "" {
			loggingService = CloudLoggingService
		}
		if *upstreamSpec.LoggingService != loggingService {
			clusterUpdate.DesiredLoggingService = loggingService
			needsUpdate = true
		}
	}
	if config.Spec.MonitoringService != nil {
		monitoringService := *config.Spec.MonitoringService
		if monitoringService == "" {
			monitoringService = CloudMonitoringService
		}
		if *upstreamSpec.MonitoringService != monitoringService {
			clusterUpdate.DesiredMonitoringService = monitoringService
			needsUpdate = true
		}
	}
	if needsUpdate {
		logrus.Infof("updating logging and monitoring configuration for cluster [%s]", config.Name)
		_, err := client.Projects.
			Locations.
			Clusters.
			Update(
				ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName),
				&gkeapi.UpdateClusterRequest{
					Update: clusterUpdate,
				},
			).Context(ctx).
			Do()
		if err != nil {
			return NotChanged, err
		}
		return Changed, nil
	}
	return NotChanged, nil
}

// UpdateNetworkPolicyEnabled updates the Cluster NetworkPolicy setting.
func UpdateNetworkPolicyEnabled(
	ctx context.Context,
	client *gkeapi.Service,
	config *gkev1.GKEClusterConfig,
	upstreamSpec *gkev1.GKEClusterConfigSpec) (Status, error) {
	if config.Spec.NetworkPolicyEnabled == nil {
		return NotChanged, nil
	}

	if *upstreamSpec.NetworkPolicyEnabled != *config.Spec.NetworkPolicyEnabled {
		logrus.Infof("updating network policy for cluster [%s]", config.Name)
		_, err := client.Projects.
			Locations.
			Clusters.
			SetNetworkPolicy(
				ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName),
				&gkeapi.SetNetworkPolicyRequest{
					NetworkPolicy: &gkeapi.NetworkPolicy{
						Enabled:  *config.Spec.NetworkPolicyEnabled,
						Provider: NetworkProviderCalico,
					},
				},
			).Context(ctx).
			Do()
		if err != nil {
			return NotChanged, err
		}
		return Changed, nil
	}
	return NotChanged, nil
}

// UpdateLocations updates Locations.
func UpdateLocations(
	ctx context.Context,
	client *gkeapi.Service,
	config *gkev1.GKEClusterConfig,
	upstreamSpec *gkev1.GKEClusterConfigSpec) (Status, error) {
	if config.Spec.Zone == "" {
		// Editing default node zones is available only in zonal clusters.
		return NotChanged, nil
	}

	clusterUpdate := &gkeapi.ClusterUpdate{}

	locations := config.Spec.Locations
	sort.Strings(locations)
	upstreamLocations := upstreamSpec.Locations
	sort.Strings(upstreamLocations)
	location := Location(config.Spec.Region, config.Spec.Zone)
	if len(locations) == 0 && len(upstreamLocations) == 1 && strings.HasPrefix(upstreamLocations[0], location) {
		// special case: no additional locations specified, upstream locations
		// was inferred from region or zone, do not try to update
		return NotChanged, nil
	}

	if !reflect.DeepEqual(locations, upstreamLocations) {
		clusterUpdate.DesiredLocations = locations
		logrus.Infof("updating locations for cluster [%s]", config.Name)
		_, err := client.Projects.
			Locations.
			Clusters.
			Update(
				ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName),
				&gkeapi.UpdateClusterRequest{
					Update: clusterUpdate,
				},
			).Context(ctx).
			Do()
		if err != nil {
			return NotChanged, err
		}
		return Changed, nil
	}

	return NotChanged, nil
}

// UpdateMaintenanceWindow updates Cluster.MaintenancePolicy.Window.DailyMaintenanceWindow.StartTime
func UpdateMaintenanceWindow(
	ctx context.Context,
	client *gkeapi.Service,
	config *gkev1.GKEClusterConfig,
	upstreamSpec *gkev1.GKEClusterConfigSpec) (Status, error) {
	if config.Spec.MaintenanceWindow == nil {
		return NotChanged, nil
	}
	window := utils.StringValue(config.Spec.MaintenanceWindow)
	if utils.StringValue(upstreamSpec.MaintenanceWindow) == window {
		return NotChanged, nil
	}

	policy := &gkeapi.MaintenancePolicy{}
	if window != "" {
		policy.Window = &gkeapi.MaintenanceWindow{
			DailyMaintenanceWindow: &gkeapi.DailyMaintenanceWindow{
				StartTime: window,
			},
		}
	}
	logrus.Infof("updating maintenance window for cluster [%s]", config.Name)
	_, err := client.Projects.
		Locations.
		Clusters.
		SetMaintenancePolicy(
			ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName),
			&gkeapi.SetMaintenancePolicyRequest{
				MaintenancePolicy: policy,
			},
		).Context(ctx).
		Do()
	if err != nil {
		return NotChanged, err
	}
	return Changed, nil
}

// UpdateNodePoolKubernetesVersionOrImageType sends a combined request to
// update either the node pool Kubernetes version or image type or both. These
// attributes are among the few that can be updated in the same request.
// If the node pool is busy, it will return a Retry status indicating the operation
// should be retried later.
func UpdateNodePoolKubernetesVersionOrImageType(
	ctx context.Context,
	client *gkeapi.Service,
	nodePool *gkev1.GKENodePoolConfig,
	config *gkev1.GKEClusterConfig,
	upstreamNodePool *gkev1.GKENodePoolConfig) (Status, error) {
	if nodePool.Version == nil {
		return NotChanged, nil
	}

	updateRequest := &gkeapi.UpdateNodePoolRequest{}
	needsUpdate := false
	npVersion := utils.StringValue(nodePool.Version)
	if npVersion != "" && utils.StringValue(upstreamNodePool.Version) != npVersion {
		logrus.Infof("updating kubernetes version on node pool [%s] on cluster [%s]", *nodePool.Name, config.Name)
		updateRequest.NodeVersion = npVersion
		needsUpdate = true
	}
	imageType := strings.ToLower(nodePool.Config.ImageType)
	if imageType != "" && strings.ToLower(upstreamNodePool.Config.ImageType) != imageType {
		logrus.Infof("updating image type on node pool [%s] on cluster [%s]", *nodePool.Name, config.Name)
		updateRequest.ImageType = imageType
		needsUpdate = true
	}
	if needsUpdate {
		_, err := client.Projects.
			Locations.
			Clusters.
			NodePools.
			Update(
				NodePoolRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName, *nodePool.Name),
				updateRequest,
			).Context(ctx).
			Do()
		if err != nil && strings.Contains(err.Error(), errWait) {
			logrus.Debugf("error %v updating node pool, will retry", err)
			return Retry, nil
		}
		if err != nil {
			return NotChanged, err
		}
		return Changed, nil
	}
	return NotChanged, nil
}

// UpdateNodePoolSize sets the size of a given node pool.
// If the node pool is busy, it will return a Retry status indicating the operation should be retried later.
func UpdateNodePoolSize(
	ctx context.Context,
	client *gkeapi.Service,
	nodePool *gkev1.GKENodePoolConfig,
	config *gkev1.GKEClusterConfig,
	upstreamNodePool *gkev1.GKENodePoolConfig) (Status, error) {
	if nodePool.InitialNodeCount == nil {
		return NotChanged, nil
	}

	if *upstreamNodePool.InitialNodeCount == *nodePool.InitialNodeCount {
		return NotChanged, nil
	}

	logrus.Infof("updating size of node pool [%s] on cluster [%s]", *nodePool.Name, config.Name)
	_, err := client.Projects.
		Locations.
		Clusters.
		NodePools.
		SetSize(
			NodePoolRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName, *nodePool.Name),
			&gkeapi.SetNodePoolSizeRequest{
				NodeCount: *nodePool.InitialNodeCount,
			},
		).Context(ctx).
		Do()
	if err != nil && strings.Contains(err.Error(), errWait) {
		logrus.Debugf("error %v updating node pool, will retry", err)
		return Retry, nil
	}
	if err != nil {
		return NotChanged, err
	}
	return Changed, nil
}

// UpdateNodePoolAutoscaling updates the autoscaling parameters for a given node pool.
// If the node pool is busy, it will return a Retry status indicating the operation should be retried later.
func UpdateNodePoolAutoscaling(
	ctx context.Context,
	client *gkeapi.Service,
	nodePool *gkev1.GKENodePoolConfig,
	config *gkev1.GKEClusterConfig,
	upstreamNodePool *gkev1.GKENodePoolConfig) (Status, error) {
	if nodePool.Autoscaling == nil {
		return NotChanged, nil
	}

	updateRequest := &gkeapi.SetNodePoolAutoscalingRequest{
		Autoscaling: &gkeapi.NodePoolAutoscaling{},
	}
	needsUpdate := false
	if upstreamNodePool.Autoscaling.Enabled != nodePool.Autoscaling.Enabled {
		updateRequest.Autoscaling.Enabled = nodePool.Autoscaling.Enabled
		needsUpdate = true
	}
	if nodePool.Autoscaling.Enabled && upstreamNodePool.Autoscaling.MaxNodeCount != nodePool.Autoscaling.MaxNodeCount {
		updateRequest.Autoscaling.Enabled = nodePool.Autoscaling.Enabled
		updateRequest.Autoscaling.MaxNodeCount = nodePool.Autoscaling.MaxNodeCount
		needsUpdate = true
	}
	if nodePool.Autoscaling.Enabled && upstreamNodePool.Autoscaling.MinNodeCount != nodePool.Autoscaling.MinNodeCount {
		updateRequest.Autoscaling.Enabled = nodePool.Autoscaling.Enabled
		updateRequest.Autoscaling.MinNodeCount = nodePool.Autoscaling.MinNodeCount
		needsUpdate = true
	}
	if needsUpdate {
		logrus.Infof("updating autoscaling config of node pool [%s] on cluster [%s]", *nodePool.Name, config.Name)
		_, err := client.Projects.
			Locations.
			Clusters.
			NodePools.
			SetAutoscaling(
				NodePoolRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName, *nodePool.Name),
				updateRequest,
			).Context(ctx).
			Do()
		if err != nil && strings.Contains(err.Error(), errWait) {
			logrus.Debugf("error %v updating node pool, will retry", err)
			return Retry, nil
		}
		if err != nil {
			return NotChanged, err
		}
		return Changed, nil
	}
	return NotChanged, nil
}

// UpdateNodePoolManagement updates the management parameters for a given node pool.
// If the node pool is busy, it will return a Retry status indicating the operation should be retried later.
func UpdateNodePoolManagement(
	ctx context.Context,
	client *gkeapi.Service,
	nodePool *gkev1.GKENodePoolConfig,
	config *gkev1.GKEClusterConfig,
	upstreamNodePool *gkev1.GKENodePoolConfig) (Status, error) {
	if nodePool.Management == nil {
		return NotChanged, nil
	}

	updateRequest := &gkeapi.SetNodePoolManagementRequest{
		Management: &gkeapi.NodeManagement{},
	}
	needsUpdate := false
	if upstreamNodePool.Management.AutoRepair != nodePool.Management.AutoRepair {
		updateRequest.Management.AutoRepair = nodePool.Management.AutoRepair
		needsUpdate = true
	}
	if upstreamNodePool.Management.AutoUpgrade != nodePool.Management.AutoUpgrade {
		updateRequest.Management.AutoUpgrade = nodePool.Management.AutoUpgrade
		needsUpdate = true
	}
	if needsUpdate {
		logrus.Infof("updating management config of node pool [%s] on cluster [%s]", *nodePool.Name, config.Name)
		_, err := client.Projects.
			Locations.
			Clusters.
			NodePools.
			SetManagement(
				NodePoolRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName, *nodePool.Name),
				updateRequest,
			).Context(ctx).
			Do()
		if err != nil && strings.Contains(err.Error(), errWait) {
			logrus.Debugf("error %v updating node pool, will retry", err)
			return Retry, nil
		}
		if err != nil {
			return NotChanged, err
		}
		return Changed, nil
	}
	return NotChanged, nil
}

func compareCidrBlockPointerSlices(lh, rh []*gkev1.GKECidrBlock) bool {
	if len(lh) != len(rh) {
		return false
	}

	lhElements := make(map[gkev1.GKECidrBlock]struct{})
	for _, v := range lh {
		if v != nil {
			lhElements[*v] = struct{}{}
		}
	}
	for _, v := range rh {
		if v == nil {
			continue
		}
		if _, ok := lhElements[*v]; !ok {
			return false
		}
	}
	return true
}
