package controller

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/rancher/gke-operator/internal/utils"
	gkev1 "github.com/rancher/gke-operator/pkg/apis/gke.cattle.io/v1"
	v12 "github.com/rancher/gke-operator/pkg/generated/controllers/gke.cattle.io/v1"
	wranglerv1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	gkeapi "google.golang.org/api/container/v1"
)

const (
	controllerName           = "gke-controller"
	controllerRemoveName     = "gke-controller-remove"
	gkeConfigCreatingPhase   = "creating"
	gkeConfigNotCreatedPhase = ""
	gkeConfigActivePhase     = "active"
	gkeConfigUpdatingPhase   = "updating"
	gkeConfigImportingPhase  = "importing"
	wait                     = 30
)

type Handler struct {
	gkeCC           v12.GKEClusterConfigClient
	gkeEnqueueAfter func(namespace, name string, duration time.Duration)
	gkeEnqueue      func(namespace, name string)
	secrets         wranglerv1.SecretClient
	secretsCache    wranglerv1.SecretCache
}

func Register(
	ctx context.Context,
	secrets wranglerv1.SecretController,
	gke v12.GKEClusterConfigController) {

	controller := &Handler{
		gkeCC:           gke,
		gkeEnqueue:      gke.Enqueue,
		gkeEnqueueAfter: gke.EnqueueAfter,
		secretsCache:    secrets.Cache(),
		secrets:         secrets,
	}

	// Register handlers
	gke.OnChange(ctx, controllerName, controller.recordError(controller.OnGkeConfigChanged))
	gke.OnRemove(ctx, controllerRemoveName, controller.OnGkeConfigRemoved)
}

func (h *Handler) OnGkeConfigChanged(key string, config *gkev1.GKEClusterConfig) (*gkev1.GKEClusterConfig, error) {
	if config == nil {
		return nil, nil
	}
	if config.DeletionTimestamp != nil {
		return nil, nil
	}

	switch config.Status.Phase {
	case gkeConfigImportingPhase:
		return h.importCluster(config)
	case gkeConfigNotCreatedPhase:
		return h.create(config)
	case gkeConfigCreatingPhase:
		return h.waitForCreationComplete(config)
	case gkeConfigActivePhase:
		return h.checkAndUpdate(config)
	case gkeConfigUpdatingPhase:
		return h.checkAndUpdate(config)
	}

	return config, nil
}

// recordError writes the error return by onChange to the failureMessage field on status. If there is no error, then
// empty string will be written to status
func (h *Handler) recordError(onChange func(key string, config *gkev1.GKEClusterConfig) (*gkev1.GKEClusterConfig, error)) func(key string, config *gkev1.GKEClusterConfig) (*gkev1.GKEClusterConfig, error) {
	return func(key string, config *gkev1.GKEClusterConfig) (*gkev1.GKEClusterConfig, error) {
		var err error
		var message string
		config, err = onChange(key, config)
		if config == nil {
			// GKE config is likely deleting
			return config, err
		}
		if config.Status.FailureMessage == message {
			return config, err
		}

		config = config.DeepCopy()

		if message != "" {
			if config.Status.Phase == gkeConfigActivePhase {
				// can assume an update is failing
				config.Status.Phase = gkeConfigUpdatingPhase
			}
		}
		config.Status.FailureMessage = message

		var recordErr error
		config, recordErr = h.gkeCC.UpdateStatus(config)
		if recordErr != nil {
			logrus.Errorf("Error recording gkecc [%s] failure message: %s", config.Name, recordErr.Error())
		}
		return config, err
	}
}

// importCluster cluster returns a spec containing the given config's displayName and region.
func (h *Handler) importCluster(config *gkev1.GKEClusterConfig) (*gkev1.GKEClusterConfig, error) {
	config.Status.Phase = gkeConfigActivePhase
	return h.gkeCC.UpdateStatus(config)
}

func (h *Handler) OnGkeConfigRemoved(key string, config *gkev1.GKEClusterConfig) (*gkev1.GKEClusterConfig, error) {
	if config.Spec.Imported {
		logrus.Infof("cluster [%s] is imported, will not delete GKE cluster", config.Name)
		return config, nil
	}
	if config.Status.Phase == gkeConfigNotCreatedPhase {
		// The most likely context here is that the cluster already existed in GKE, so we shouldn't delete it
		logrus.Warnf("cluster [%s] never advanced to creating status, will not delete GKE cluster", config.Name)
		return config, nil
	}

	logrus.Infof("deleting cluster [%s]", config.Name)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc, err := utils.GetServiceClient(ctx, config.Spec.CredentialContent)
	if err != nil {
		return config, err
	}

	logrus.Debugf("Removing cluster %v from project %v, region/zone %v", config.Spec.ClusterName, config.Spec.ProjectID, config.Spec.Region)
	operation, err := utils.WaitClusterRemoveExp(ctx, svc, config)
	if err != nil && !strings.Contains(err.Error(), "notFound") {
		return config, err
	} else if err == nil {
		logrus.Debugf("Cluster %v delete is called. Status Code %v", config.Spec.ClusterName, operation.HTTPStatusCode)
	} else {
		logrus.Debugf("Cluster %s doesn't exist", config.Spec.ClusterName)
	}

	return config, nil
}

func (h *Handler) create(config *gkev1.GKEClusterConfig) (*gkev1.GKEClusterConfig, error) {
	if config.Spec.Imported {
		config = config.DeepCopy()
		config.Status.Phase = gkeConfigImportingPhase
		return h.gkeCC.UpdateStatus(config)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc, err := utils.GetServiceClient(ctx, config.Spec.CredentialContent)
	if err != nil {
		return config, err
	}

	createClusterRequest, err := utils.GenerateGkeClusterCreateRequest(config)
	if err != nil {
		return config, err
	}

	operation, err := svc.Projects.Locations.Clusters.Create(
		utils.LocationRRN(config.Spec.ProjectID, config.Spec.Region),
		createClusterRequest).Context(ctx).Do()

	if err != nil {
		return config, err
	}
	logrus.Debugf("Cluster %s create is called for project %s and region/zone %s. Status Code %v",
		config.Spec.ClusterName, config.Spec.ProjectID, config.Spec.Region, operation.HTTPStatusCode)

	config = config.DeepCopy()
	config.Status.Phase = gkeConfigCreatingPhase
	return h.gkeCC.UpdateStatus(config)
}

func (h *Handler) checkAndUpdate(config *gkev1.GKEClusterConfig) (*gkev1.GKEClusterConfig, error) {
	if err := h.validateUpdate(config); err != nil {
		config = config.DeepCopy()
		config.Status.Phase = gkeConfigUpdatingPhase
		var updateErr error
		config, updateErr = h.gkeCC.UpdateStatus(config)
		if updateErr != nil {
			return config, updateErr
		}
		return config, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc, err := utils.GetServiceClient(ctx, config.Spec.CredentialContent)
	if err != nil {
		return config, err
	}

	cluster, err := svc.Projects.Locations.Clusters.Get(utils.ClusterRRN(config.Spec.ProjectID, config.Spec.Region, config.Spec.ClusterName)).Context(ctx).Do()
	if err != nil {
		return config, err
	}

	if cluster.Status == utils.ClusterStatusReconciling {
		// upstream cluster is already updating, must wait until sending next update
		logrus.Infof("waiting for cluster [%s] to finish updating", config.Name)
		if config.Status.Phase != gkeConfigUpdatingPhase {
			config = config.DeepCopy()
			config.Status.Phase = gkeConfigUpdatingPhase
			return h.gkeCC.UpdateStatus(config)
		}
		h.gkeEnqueueAfter(config.Namespace, config.Name, 30*time.Second)
		return config, nil
	}

	for _, np := range cluster.NodePools {

		if status := np.Status; status == utils.NodePoolStatusReconciling || status == utils.NodePoolStatusStopping ||
			status == utils.NodePoolStatusProvisioning {
			if config.Status.Phase != gkeConfigUpdatingPhase {
				config = config.DeepCopy()
				config.Status.Phase = gkeConfigUpdatingPhase
				config, err = h.gkeCC.UpdateStatus(config)
				if err != nil {
					return config, err
				}
			}
			logrus.Infof("waiting for cluster [%s] to update nodegroups [%s]", config.Name, np.Name)
			h.gkeEnqueueAfter(config.Namespace, config.Name, 30*time.Second)
			return config, nil
		}
	}

	upstreamSpec, err := utils.BuildUpstreamClusterState(cluster)
	if err != nil {
		return config, err
	}

	return h.updateUpstreamClusterState(config, upstreamSpec, svc)
}

// enqueueUpdate enqueues the config if it is already in the updating phase. Otherwise, the
// phase is updated to "updating". This is important because the object needs to reenter the
// onChange handler to start waiting on the update.
func (h *Handler) enqueueUpdate(config *gkev1.GKEClusterConfig) (*gkev1.GKEClusterConfig, error) {
	if config.Status.Phase == gkeConfigUpdatingPhase {
		h.gkeEnqueue(config.Namespace, config.Name)
		return config, nil
	}
	config = config.DeepCopy()
	config.Status.Phase = gkeConfigUpdatingPhase
	return h.gkeCC.UpdateStatus(config)
}

func (h *Handler) updateUpstreamClusterState(config *gkev1.GKEClusterConfig, upstreamSpec *gkev1.GKEClusterConfigSpec, svc *gkeapi.Service) (*gkev1.GKEClusterConfig, error) {

	if config.Spec.KubernetesVersion != nil {
		if utils.StringValue(upstreamSpec.KubernetesVersion) != utils.StringValue(config.Spec.KubernetesVersion) {
			logrus.Infof("updating kubernetes version for cluster [%s]", config.Name)
			if err := utils.UpdateCluster(config, &gkeapi.UpdateClusterRequest{
				Update: &gkeapi.ClusterUpdate{
					DesiredMasterVersion: *config.Spec.KubernetesVersion,
				}}); err != nil {
				return config, err
			}
			return h.enqueueUpdate(config)
		}
	}

	clusterUpdate := &gkeapi.ClusterUpdate{}

	addons := config.Spec.ClusterAddons
	addonsNeedUpdate := false
	if addons != nil {
		if upstreamSpec.ClusterAddons.HTTPLoadBalancing != addons.HTTPLoadBalancing {
			clusterUpdate.DesiredAddonsConfig = &gkeapi.AddonsConfig{}
			clusterUpdate.DesiredAddonsConfig.HttpLoadBalancing = &gkeapi.HttpLoadBalancing{
				Disabled: !addons.HTTPLoadBalancing,
			}
			addonsNeedUpdate = true
		}
		if upstreamSpec.ClusterAddons.HorizontalPodAutoscaling != addons.HorizontalPodAutoscaling {
			if clusterUpdate.DesiredAddonsConfig == nil {
				clusterUpdate.DesiredAddonsConfig = &gkeapi.AddonsConfig{}
			}
			clusterUpdate.DesiredAddonsConfig.HorizontalPodAutoscaling = &gkeapi.HorizontalPodAutoscaling{
				Disabled: !addons.HorizontalPodAutoscaling,
			}
			addonsNeedUpdate = true
		}
		if upstreamSpec.ClusterAddons.NetworkPolicyConfig != addons.NetworkPolicyConfig {
			// If disabling NetworkPolicyConfig for the cluster, NetworkPolicyEnabled
			// (which affects nodes) needs to be disabled first. If
			// NetworkPolicyEnabled is already set to false downstream but not yet
			// complete upstream, that update will be enqueued later in this
			// sequence and we just need to wait for it to complete.
			if !addons.NetworkPolicyConfig && (config.Spec.NetworkPolicyEnabled != nil && !*config.Spec.NetworkPolicyEnabled) && *upstreamSpec.NetworkPolicyEnabled {
				logrus.Infof("waiting to update NetworkPolicyConfig cluster addon")
			} else {
				if clusterUpdate.DesiredAddonsConfig == nil {
					clusterUpdate.DesiredAddonsConfig = &gkeapi.AddonsConfig{}
				}
				clusterUpdate.DesiredAddonsConfig.NetworkPolicyConfig = &gkeapi.NetworkPolicyConfig{
					Disabled: !addons.NetworkPolicyConfig,
				}
				addonsNeedUpdate = true
			}
		}
	}
	if addonsNeedUpdate {
		logrus.Infof("updating addon configuration for cluster [%s]", config.Name)
		err := utils.UpdateCluster(config, &gkeapi.UpdateClusterRequest{
			Update: clusterUpdate,
		})
		// In the case of disabling both NetworkPolicyEnabled and NetworkPolicyConfig,
		// the node pool will automatically be recreated after NetworkPolicyEnabled is
		// disabled, so we need to wait. Capture this event and log it as Info
		// rather than an error, since this is a normal but potentially
		// time-consuming event.
		if err != nil {
			matched, matchErr := regexp.MatchString(`Node pool "\S+" requires recreation`, err.Error())
			if matchErr != nil {
				return config, fmt.Errorf("programming error: %w", matchErr)
			}
			if matched {
				logrus.Infof("waiting for node pool to finish recreation")
				h.gkeEnqueueAfter(config.Namespace, config.Name, wait*time.Second)
				return config, nil
			}
			return config, err
		}
		return h.enqueueUpdate(config)
	}

	clusterUpdate = &gkeapi.ClusterUpdate{}
	netconfigNeedsUpdate := false
	if config.Spec.MasterAuthorizedNetworksConfig != nil {
		if upstreamSpec.MasterAuthorizedNetworksConfig.Enabled != config.Spec.MasterAuthorizedNetworksConfig.Enabled {
			clusterUpdate.DesiredMasterAuthorizedNetworksConfig = &gkeapi.MasterAuthorizedNetworksConfig{
				Enabled: config.Spec.MasterAuthorizedNetworksConfig.Enabled,
			}
			netconfigNeedsUpdate = true
		}
		if config.Spec.MasterAuthorizedNetworksConfig.Enabled && !utils.CompareCidrBlockPointerSlices(
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
			netconfigNeedsUpdate = true
		}
	}
	if netconfigNeedsUpdate {
		logrus.Infof("updating master authorized networks configuration for cluster [%s]", config.Name)
		if err := utils.UpdateCluster(config, &gkeapi.UpdateClusterRequest{
			Update: clusterUpdate,
		}); err != nil {
			return config, err
		}
		return h.enqueueUpdate(config)
	}

	clusterUpdate = &gkeapi.ClusterUpdate{}
	loggingOrMonitoringNeedsUpdate := false
	if config.Spec.LoggingService != nil {
		loggingService := *config.Spec.LoggingService
		if loggingService == "" {
			loggingService = utils.CloudLoggingService
		}
		if *upstreamSpec.LoggingService != loggingService {
			clusterUpdate.DesiredLoggingService = loggingService
			loggingOrMonitoringNeedsUpdate = true
		}
	}
	if config.Spec.MonitoringService != nil {
		monitoringService := *config.Spec.MonitoringService
		if monitoringService == "" {
			monitoringService = utils.CloudMonitoringService
		}
		if *upstreamSpec.MonitoringService != monitoringService {
			clusterUpdate.DesiredMonitoringService = monitoringService
			loggingOrMonitoringNeedsUpdate = true
		}
	}
	if loggingOrMonitoringNeedsUpdate {
		logrus.Infof("updating logging and monitoring configuration for cluster [%s]", config.Name)
		if err := utils.UpdateCluster(config, &gkeapi.UpdateClusterRequest{
			Update: clusterUpdate,
		}); err != nil {
			return config, err
		}
		return h.enqueueUpdate(config)
	}

	if config.Spec.NetworkPolicyEnabled != nil {
		if *upstreamSpec.NetworkPolicyEnabled != *config.Spec.NetworkPolicyEnabled {
			logrus.Infof("updating network policy for cluster [%s]", config.Name)
			if err := utils.UpdateNetworkPolicy(config, &gkeapi.SetNetworkPolicyRequest{
				NetworkPolicy: &gkeapi.NetworkPolicy{
					Enabled:  *config.Spec.NetworkPolicyEnabled,
					Provider: utils.NetworkProviderCalico,
				},
			}); err != nil {
				return config, err
			}
			return h.enqueueUpdate(config)
		}
	}

	if config.Spec.NodePools != nil {
		updateNodePoolRequest := &gkeapi.UpdateNodePoolRequest{}
		needsUpdate := false
		upstreamNodePools := buildNodePoolMap(upstreamSpec.NodePools)
		for _, np := range config.Spec.NodePools {
			upstreamNodePool := upstreamNodePools[*np.Name]
			if utils.StringValue(upstreamNodePool.Version) != utils.StringValue(np.Version) {
				updateNodePoolRequest.NodeVersion = *np.Version
				needsUpdate = true
			}
			if strings.ToLower(upstreamNodePool.Config.ImageType) != strings.ToLower(np.Config.ImageType) {
				updateNodePoolRequest.ImageType = np.Config.ImageType
				needsUpdate = true
			}
			if needsUpdate {
				logrus.Infof("updating nodepool [%s] on cluster [%s]", *np.Name, config.Name)
				if err := utils.UpdateNodePool(*np.Name, config, updateNodePoolRequest); err != nil {
					return config, err
				}
				return h.enqueueUpdate(config)
			}
			if *upstreamNodePool.InitialNodeCount != *np.InitialNodeCount {
				logrus.Infof("updating size of nodepool [%s] on cluster [%s]", *np.Name, config.Name)
				if err := utils.SetNodePoolSize(*np.Name, config, &gkeapi.SetNodePoolSizeRequest{
					NodeCount: *np.InitialNodeCount,
				}); err != nil {
					return config, err
				}
				return h.enqueueUpdate(config)
			}
			autoscalingRequest := &gkeapi.SetNodePoolAutoscalingRequest{
				Autoscaling: &gkeapi.NodePoolAutoscaling{},
			}
			needsUpdate := false
			if np.Autoscaling != nil {
				if upstreamNodePool.Autoscaling.Enabled != np.Autoscaling.Enabled {
					autoscalingRequest.Autoscaling.Enabled = np.Autoscaling.Enabled
					needsUpdate = true
				}
				if np.Autoscaling.Enabled && upstreamNodePool.Autoscaling.MaxNodeCount != np.Autoscaling.MaxNodeCount {
					autoscalingRequest.Autoscaling.MaxNodeCount = np.Autoscaling.MaxNodeCount
					needsUpdate = true
				}
				if np.Autoscaling.Enabled && upstreamNodePool.Autoscaling.MinNodeCount != np.Autoscaling.MinNodeCount {
					autoscalingRequest.Autoscaling.MinNodeCount = np.Autoscaling.MinNodeCount
					needsUpdate = true
				}
			}
			if needsUpdate {
				logrus.Infof("updating autoscaling config of nodepool [%s] on cluster [%s]", *np.Name, config.Name)
				if err := utils.SetNodePoolAutoscaling(*np.Name, config, autoscalingRequest); err != nil {
					return config, err
				}
				return h.enqueueUpdate(config)
			}
		}
	}

	// no new updates, set to active
	if config.Status.Phase != gkeConfigActivePhase {
		logrus.Infof("cluster [%s] finished updating", config.Name)
		config = config.DeepCopy()
		config.Status.Phase = gkeConfigActivePhase
		return h.gkeCC.UpdateStatus(config)
	}

	return config, nil
}

func (h *Handler) waitForCreationComplete(config *gkev1.GKEClusterConfig) (*gkev1.GKEClusterConfig, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc, err := utils.GetServiceClient(ctx, config.Spec.CredentialContent)
	if err != nil {
		return config, err
	}
	cluster, err := svc.Projects.Locations.Clusters.Get(utils.ClusterRRN(config.Spec.ProjectID, config.Spec.Region, config.Spec.ClusterName)).Context(ctx).Do()
	if err != nil {
		return config, err
	}
	if cluster.Status == utils.ClusterStatusError {
		return config, fmt.Errorf("creation failed for cluster %v", config.Spec.ClusterName)
	}
	if cluster.Status == utils.ClusterStatusRunning {
		logrus.Infof("Cluster %v is running", config.Spec.ClusterName)
		config = config.DeepCopy()
		config.Status.Phase = gkeConfigActivePhase
		return h.gkeCC.UpdateStatus(config)
	}
	logrus.Infof("waiting for cluster [%s] to finish creating", config.Name)
	h.gkeEnqueueAfter(config.Namespace, config.Name, wait*time.Second)

	return config, nil
}

func (h *Handler) validateUpdate(config *gkev1.GKEClusterConfig) error {

	var clusterVersion *semver.Version
	if config.Spec.KubernetesVersion != nil {
		var err error
		clusterVersion, err = semver.New(fmt.Sprintf("%s.0", utils.StringValue(config.Spec.KubernetesVersion)))
		if err != nil {
			return fmt.Errorf("improper version format for cluster [%s]: %s", config.Name, utils.StringValue(config.Spec.KubernetesVersion))
		}
	}

	var errors []string
	// validate nodegroup versions
	for _, np := range config.Spec.NodePools {
		if np.Version == nil {
			continue
		}
		version, err := semver.New(fmt.Sprintf("%s.0", utils.StringValue(np.Version)))
		if err != nil {
			errors = append(errors, fmt.Sprintf("improper version format for nodegroup [%s]: %s", utils.StringValue(np.Name), utils.StringValue(np.Version)))
			continue
		}
		if clusterVersion == nil {
			continue
		}
		if clusterVersion.EQ(*version) {
			continue
		}
		if clusterVersion.Minor-version.Minor == 1 {
			continue
		}
		errors = append(errors, fmt.Sprintf("versions for cluster [%s] and nodegroup [%s] not compatible: all nodegroup kubernetes versions"+
			"must be equal to or one minor version lower than the cluster kubernetes version", utils.StringValue(config.Spec.KubernetesVersion), utils.StringValue(np.Version)))
	}
	if len(errors) != 0 {
		return fmt.Errorf(strings.Join(errors, ";"))
	}
	return nil
}

func buildNodePoolMap(nodePools []gkev1.NodePoolConfig) map[string]*gkev1.NodePoolConfig {
	ret := make(map[string]*gkev1.NodePoolConfig)
	for i := range nodePools {
		if nodePools[i].Name != nil {
			ret[*nodePools[i].Name] = &nodePools[i]
		}
	}
	return ret
}
