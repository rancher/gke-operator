package gke

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"
	gkeapi "google.golang.org/api/container/v1"

	gkev1 "github.com/rancher/gke-operator/pkg/apis/gke.cattle.io/v1"
	"github.com/rancher/gke-operator/pkg/gke/services"
	"github.com/rancher/gke-operator/pkg/utils"
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
func UpdateMasterKubernetesVersion(ctx context.Context, gkeClient services.GKEClusterService, config *gkev1.GKEClusterConfig, upstreamSpec *gkev1.GKEClusterConfigSpec) (Status, error) {
	kubeVersion := utils.StringValue(config.Spec.KubernetesVersion)
	if kubeVersion == "" {
		return NotChanged, nil
	}

	if utils.StringValue(upstreamSpec.KubernetesVersion) == kubeVersion {
		return NotChanged, nil
	}

	logrus.Infof("updating kubernetes version from %s to %s for cluster [%s (%s)]", utils.StringValue(upstreamSpec.KubernetesVersion), kubeVersion, config.Spec.ClusterName, config.Name)
	_, err := gkeClient.ClusterUpdate(ctx,
		ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName),
		&gkeapi.UpdateClusterRequest{
			Update: &gkeapi.ClusterUpdate{
				DesiredMasterVersion: kubeVersion,
			},
		},
	)
	if err != nil {
		return NotChanged, err
	}
	return Changed, nil
}

// UpdateClusterAddons updates the cluster addons.
// In the case of the NetworkPolicyConfig addon, this may need to be retried after NetworkPolicyEnabled has been updated.
func UpdateClusterAddons(ctx context.Context, gkeClient services.GKEClusterService, config *gkev1.GKEClusterConfig, upstreamSpec *gkev1.GKEClusterConfigSpec) (Status, error) {
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
		logrus.Infof("updating addon configuration (upstream: %+v; config: %+v) for cluster [%s (%s)]", upstreamSpec.ClusterAddons, config.Spec.ClusterAddons, config.Spec.ClusterName, config.Name)
		_, err := gkeClient.ClusterUpdate(ctx,
			ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName),
			&gkeapi.UpdateClusterRequest{
				Update: clusterUpdate,
			},
		)
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
	gkeClient services.GKEClusterService,
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
		logrus.Infof("updating master authorized networks configuration (upstream: %+v; config: %+v) for cluster [%s (%s)]", upstreamSpec.MasterAuthorizedNetworksConfig, config.Spec.MasterAuthorizedNetworksConfig, config.Spec.ClusterName, config.Name)
		_, err := gkeClient.ClusterUpdate(ctx,
			ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName),
			&gkeapi.UpdateClusterRequest{
				Update: clusterUpdate,
			},
		)
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
	gkeClient services.GKEClusterService,
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
		logrus.Infof("updating logging (upstream: %s; config: %s) and monitoring (upstream: %s; config: %s) configuration for cluster [%s (%s)]", *upstreamSpec.MonitoringService, *config.Spec.MonitoringService, *upstreamSpec.LoggingService, *config.Spec.LoggingService, config.Spec.ClusterName, config.Name)
		_, err := gkeClient.ClusterUpdate(ctx,
			ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName),
			&gkeapi.UpdateClusterRequest{
				Update: clusterUpdate,
			},
		)
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
	gkeClient services.GKEClusterService,
	config *gkev1.GKEClusterConfig,
	upstreamSpec *gkev1.GKEClusterConfigSpec) (Status, error) {
	if config.Spec.NetworkPolicyEnabled == nil {
		return NotChanged, nil
	}

	if *upstreamSpec.NetworkPolicyEnabled != *config.Spec.NetworkPolicyEnabled {
		logrus.Infof("updating network policy (upstream: %v; config: %v) for cluster [%s (%s)]", *upstreamSpec.NetworkPolicyEnabled, *config.Spec.NetworkPolicyEnabled, config.Spec.ClusterName, config.Name)
		_, err := gkeClient.SetNetworkPolicy(ctx,
			ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName),
			&gkeapi.SetNetworkPolicyRequest{
				NetworkPolicy: &gkeapi.NetworkPolicy{
					Enabled:  *config.Spec.NetworkPolicyEnabled,
					Provider: NetworkProviderCalico,
				},
			},
		)
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
	gkeClient services.GKEClusterService,
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
		logrus.Infof("updating locations (upstream: %v; config: %v) for cluster [%s (%s)]", upstreamLocations, locations, config.Spec.ClusterName, config.Name)
		_, err := gkeClient.ClusterUpdate(ctx,
			ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName),
			&gkeapi.UpdateClusterRequest{
				Update: clusterUpdate,
			},
		)
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
	gkeClient services.GKEClusterService,
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
	logrus.Infof("updating maintenance window from %s to %s for cluster [%s (%s)]", *upstreamSpec.MaintenanceWindow, *config.Spec.MaintenanceWindow, config.Spec.ClusterName, config.Name)
	_, err := gkeClient.SetMaintenancePolicy(ctx,
		ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName),
		&gkeapi.SetMaintenancePolicyRequest{
			MaintenancePolicy: policy,
		},
	)
	if err != nil {
		return NotChanged, err
	}
	return Changed, nil
}

// UpdateLabels updates the cluster labels.
func UpdateLabels(
	ctx context.Context,
	gkeClient services.GKEClusterService,
	config *gkev1.GKEClusterConfig,
	upstreamSpec *gkev1.GKEClusterConfigSpec) (Status, error) {
	if config.Spec.Labels == nil || reflect.DeepEqual(config.Spec.Labels, upstreamSpec.Labels) || (upstreamSpec.Labels == nil && len(config.Spec.Labels) == 0) {
		return NotChanged, nil
	}
	cluster, err := GetCluster(ctx, gkeClient, &config.Spec)
	if err != nil {
		return NotChanged, err
	}
	logrus.Infof("updating cluster labels (upstream: %+v; config: %+v) for cluster [%s (%s)]", upstreamSpec.Labels, config.Spec.Labels, config.Spec.ClusterName, config.Name)
	_, err = gkeClient.SetResourceLabels(ctx,
		ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName),
		&gkeapi.SetLabelsRequest{
			LabelFingerprint: cluster.LabelFingerprint,
			ResourceLabels:   config.Spec.Labels,
		},
	)
	if err != nil {
		if strings.Contains(err.Error(), "Labels could not be set due to fingerprint mismatch") {
			logrus.Debugf("got transient error: %v, retrying", err)
			return Retry, nil
		}
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
	gkeClient services.GKEClusterService,
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
		logrus.Infof("updating kubernetes version from %s to %s on node pool [%s] on cluster [%s (%s)]", npVersion, utils.StringValue(upstreamNodePool.Version), *nodePool.Name, config.Spec.ClusterName, config.Name)
		updateRequest.NodeVersion = npVersion
		needsUpdate = true
	}
	imageType := strings.ToLower(nodePool.Config.ImageType)
	if imageType != "" && strings.ToLower(upstreamNodePool.Config.ImageType) != imageType {
		logrus.Infof("updating image type on node pool [%s] from %s to %s on cluster [%s (%s)]", imageType, upstreamNodePool.Config.ImageType, *nodePool.Name, config.Spec.ClusterName, config.Name)
		updateRequest.ImageType = imageType
		needsUpdate = true
	}
	if needsUpdate {
		_, err := gkeClient.NodePoolUpdate(ctx,
			NodePoolRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName, *nodePool.Name),
			updateRequest)
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
	gkeClient services.GKEClusterService,
	nodePool *gkev1.GKENodePoolConfig,
	config *gkev1.GKEClusterConfig,
	upstreamNodePool *gkev1.GKENodePoolConfig) (Status, error) {
	if nodePool.InitialNodeCount == nil {
		return NotChanged, nil
	}

	if *upstreamNodePool.InitialNodeCount == *nodePool.InitialNodeCount {
		return NotChanged, nil
	}

	logrus.Infof("updating size of node pool [%s] from %d to %d on cluster [%s (%s)]", *nodePool.Name, *upstreamNodePool.InitialNodeCount, *nodePool.InitialNodeCount, config.Spec.ClusterName, config.Name)
	_, err := gkeClient.SetSize(ctx,
		NodePoolRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName, *nodePool.Name),
		&gkeapi.SetNodePoolSizeRequest{
			NodeCount: *nodePool.InitialNodeCount,
		},
	)
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
	gkeClient services.GKEClusterService,
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
		logrus.Infof("updating autoscaling config (upstream: %+v; config: %+v) of node pool [%s] on cluster [%s (%s)]", upstreamNodePool.Autoscaling, nodePool.Autoscaling, *nodePool.Name, config.Spec.ClusterName, config.Name)
		_, err := gkeClient.SetAutoscaling(ctx,
			NodePoolRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName, *nodePool.Name),
			updateRequest)
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
	gkeClient services.GKEClusterService,
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
		logrus.Infof("updating management config (upstream: %+v, config: %+v) of node pool [%s] on cluster [%s (%s)]", upstreamNodePool.Management, nodePool.Management, *nodePool.Name, config.Spec.ClusterName, config.Name)
		_, err := gkeClient.SetManagement(ctx,
			NodePoolRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName, *nodePool.Name),
			updateRequest)
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

// UpdateNodePoolConfig updates the configured parameters for a given node pool.
// If the node pool is busy, it will return a Retry status indicating the operation should be retried later.
func UpdateNodePoolConfig(
	ctx context.Context,
	gkeClient services.GKEClusterService,
	nodePool *gkev1.GKENodePoolConfig,
	config *gkev1.GKEClusterConfig,
	upstreamNodePool *gkev1.GKENodePoolConfig) (Status, error) {
	if nodePool.Config.Labels == nil || reflect.DeepEqual(nodePool.Config.Labels, upstreamNodePool.Config.Labels) || (upstreamNodePool.Config.Labels == nil && len(nodePool.Config.Labels) == 0) {
		return NotChanged, nil
	}

	updateRequest := &gkeapi.UpdateNodePoolRequest{
		Labels: &gkeapi.NodeLabels{
			Labels: nodePool.Config.Labels,
		},
	}

	logrus.Infof("updating config for node pool [%s] on cluster [%s (%s)]", *nodePool.Name, config.Spec.ClusterName, config.Name)
	_, err := gkeClient.NodePoolUpdate(ctx,
		NodePoolRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName, *nodePool.Name),
		updateRequest)
	if err != nil {
		if strings.Contains(err.Error(), errWait) {
			logrus.Debugf("error %v updating node pool, will retry", err)
			return Retry, nil
		}
		return NotChanged, err
	}
	return Changed, nil
}

func GetCluster(ctx context.Context, gkeClient services.GKEClusterService, configSpec *gkev1.GKEClusterConfigSpec) (*gkeapi.Cluster, error) {
	return gkeClient.ClusterGet(ctx,
		ClusterRRN(configSpec.ProjectID, Location(configSpec.Region, configSpec.Zone), configSpec.ClusterName))
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
