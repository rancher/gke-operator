package controller

import (
	"context"
	"fmt"
	"time"

	gkev1 "github.com/rancher/gke-operator/pkg/apis/gke.cattle.io/v1"
	gkecontrollers "github.com/rancher/gke-operator/pkg/generated/controllers/gke.cattle.io/v1"
	"github.com/rancher/gke-operator/pkg/gke"

	wranglerv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"

	"github.com/rancher/gke-operator/pkg/gke/services"

	gkeapi "google.golang.org/api/container/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	gkeClusterConfigKind     = "GKEClusterConfig"
	controllerName           = "gke-controller"
	controllerRemoveName     = "gke-controller-remove"
	gkeConfigCreatingPhase   = "creating"
	gkeConfigNotCreatedPhase = ""
	gkeConfigActivePhase     = "active"
	gkeConfigUpdatingPhase   = "updating"
	gkeConfigImportingPhase  = "importing"
	wait                     = 30
)

// Cluster Status
const (
	// ClusterStatusRunning The RUNNING state indicates the cluster has been
	// created and is fully usable
	ClusterStatusRunning = "RUNNING"

	// ClusterStatusError The ERROR state indicates the cluster is unusable
	ClusterStatusError = "ERROR"

	// ClusterStatusReconciling The RECONCILING state indicates that some work is
	// actively being done on the cluster, such as upgrading the master or
	// node software.
	ClusterStatusReconciling = "RECONCILING"
)

// Node Pool Status
const (
	// NodePoolStatusProvisioning The PROVISIONING state indicates the node pool is
	// being created.
	NodePoolStatusProvisioning = "PROVISIONING"

	// NodePoolStatusReconciling The RECONCILING state indicates that some work is
	// actively being done on the node pool, such as upgrading node software.
	NodePoolStatusReconciling = "RECONCILING"

	//  NodePoolStatusStopping The STOPPING state indicates the node pool is being
	// deleted.
	NodePoolStatusStopping = "STOPPING"
)

type Handler struct {
	gkeCC           gkecontrollers.GKEClusterConfigClient
	gkeEnqueueAfter func(namespace, name string, duration time.Duration)
	gkeEnqueue      func(namespace, name string)
	secrets         wranglerv1.SecretClient
	secretsCache    wranglerv1.SecretCache
	gkeClient       services.GKEClusterService
	gkeClientCtx    context.Context
}

func Register(
	ctx context.Context,
	secrets wranglerv1.SecretController,
	gke gkecontrollers.GKEClusterConfigController) {
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

func (h *Handler) OnGkeConfigChanged(_ string, config *gkev1.GKEClusterConfig) (*gkev1.GKEClusterConfig, error) {
	if config == nil {
		return nil, nil
	}

	if config.DeletionTimestamp != nil {
		return nil, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h.gkeClientCtx = ctx

	cred, err := GetSecret(h.gkeClientCtx, h.secrets, &config.Spec)
	if err != nil {
		return config, err
	}

	gkeClient, err := gke.GetGKEClusterClient(h.gkeClientCtx, cred)
	if err != nil {
		return config, err
	}

	h.gkeClient = gkeClient

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
		if err != nil && errors.IsConflict(err) {
			// If a conflict error happened on an UpdateStatus call, it is probably because the handler was working from an out of sync cached object.
			// UpdateStatus returns an empty struct when it fails, so trying to run UpdateStatus again to record the failureMessage will also fail.
			// Just return the error instead of attempting to update the failure message.
			return nil, err
		}
		if err != nil {
			message = err.Error()
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
			logrus.Errorf("Error recording gkecc [%s (id: %s)] failure message: %s, original error: %s", config.Spec.ClusterName, config.Name, recordErr, err)
		}
		return config, err
	}
}

// importCluster returns an active cluster spec containing the given config's clusterName and region/zone
// and creates a Secret containing the cluster's CA and endpoint retrieved from the cluster object.
func (h *Handler) importCluster(config *gkev1.GKEClusterConfig) (*gkev1.GKEClusterConfig, error) {
	cluster, err := gke.GetCluster(h.gkeClientCtx, h.gkeClient, &config.Spec)
	if err != nil {
		return config, err
	}
	if err := h.createCASecret(config, cluster); err != nil {
		return config, err
	}

	config.Status.Phase = gkeConfigActivePhase
	return h.gkeCC.UpdateStatus(config)
}

func (h *Handler) OnGkeConfigRemoved(_ string, config *gkev1.GKEClusterConfig) (*gkev1.GKEClusterConfig, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h.gkeClientCtx = ctx

	cred, err := GetSecret(h.gkeClientCtx, h.secrets, &config.Spec)
	if err != nil {
		return config, err
	}

	gkeClient, err := gke.GetGKEClusterClient(h.gkeClientCtx, cred)
	if err != nil {
		return config, err
	}

	h.gkeClient = gkeClient

	if config.Spec.Imported {
		logrus.Infof("Cluster [%s (id: %s)] is imported, will not delete GKE cluster", config.Spec.ClusterName, config.Name)
		return config, nil
	}
	if config.Status.Phase == gkeConfigNotCreatedPhase {
		// The most likely context here is that the cluster already existed in GKE, so we shouldn't delete it
		logrus.Warnf("Cluster [%s (id: %s)] never advanced to creating status, will not delete GKE cluster", config.Spec.ClusterName, config.Name)
		return config, nil
	}

	logrus.Infof("Removing cluster [%s (id: %s)] from project %s, region/zone %s", config.Spec.ClusterName, config.Name, config.Spec.ProjectID, gke.Location(config.Spec.Region, config.Spec.Zone))
	if err := gke.RemoveCluster(h.gkeClientCtx, h.gkeClient, config); err != nil {
		logrus.Debugf("Error deleting cluster %s: %v", config.Spec.ClusterName, err)
		return config, err
	}

	return config, nil
}

func (h *Handler) create(config *gkev1.GKEClusterConfig) (*gkev1.GKEClusterConfig, error) {
	if config.Spec.Imported {
		logrus.Infof("Importing cluster [%s (id: %s)]", config.Spec.ClusterName, config.Name)
		config = config.DeepCopy()
		config.Status.Phase = gkeConfigImportingPhase
		return h.gkeCC.UpdateStatus(config)
	}

	if err := gke.Create(h.gkeClientCtx, h.gkeClient, config); err != nil {
		return config, err
	}

	config = config.DeepCopy()
	config.Status.Phase = gkeConfigCreatingPhase
	return h.gkeCC.UpdateStatus(config)
}

func (h *Handler) checkAndUpdate(config *gkev1.GKEClusterConfig) (*gkev1.GKEClusterConfig, error) {
	cluster, err := gke.GetCluster(h.gkeClientCtx, h.gkeClient, &config.Spec)
	if err != nil {
		return config, err
	}

	if cluster.Status == ClusterStatusReconciling {
		// upstream cluster is already updating, must wait until sending next update
		logrus.Infof("Waiting for cluster [%s (id: %s)] to finish updating", config.Spec.ClusterName, config.Name)
		if config.Status.Phase != gkeConfigUpdatingPhase {
			config = config.DeepCopy()
			config.Status.Phase = gkeConfigUpdatingPhase
			return h.gkeCC.UpdateStatus(config)
		}
		h.gkeEnqueueAfter(config.Namespace, config.Name, 30*time.Second)
		return config, nil
	}

	for _, np := range cluster.NodePools {
		if status := np.Status; status == NodePoolStatusReconciling || status == NodePoolStatusStopping ||
			status == NodePoolStatusProvisioning {
			if config.Status.Phase != gkeConfigUpdatingPhase {
				config = config.DeepCopy()
				config.Status.Phase = gkeConfigUpdatingPhase
				config, err = h.gkeCC.UpdateStatus(config)
				if err != nil {
					return config, err
				}
			}
			logrus.Infof("Waiting for cluster [%s (id: %s)] to update node pool [%s]", config.Spec.ClusterName, config.Name, np.Name)
			h.gkeEnqueueAfter(config.Namespace, config.Name, 30*time.Second)
			return config, nil
		}
	}

	upstreamSpec, err := h.buildUpstreamClusterState(cluster)
	if err != nil {
		return config, err
	}

	return h.updateUpstreamClusterState(config, upstreamSpec)
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

func (h *Handler) updateUpstreamClusterState(config *gkev1.GKEClusterConfig, upstreamSpec *gkev1.GKEClusterConfigSpec) (*gkev1.GKEClusterConfig, error) {
	updateConfig := false
	configCopy := config.DeepCopy()

	if upstreamSpec.EnableDataplaneV2 != nil {
		if configCopy.Spec.EnableDataplaneV2 == nil || *configCopy.Spec.EnableDataplaneV2 != *upstreamSpec.EnableDataplaneV2 {
			configCopy.Spec.EnableDataplaneV2 = upstreamSpec.EnableDataplaneV2
			updateConfig = true
		}
	}

	if upstreamSpec.IPAllocationPolicy != nil {
		if configCopy.Spec.IPAllocationPolicy == nil {
			configCopy.Spec.IPAllocationPolicy = &gkev1.GKEIPAllocationPolicy{}
		}

		if configCopy.Spec.IPAllocationPolicy.StackType != upstreamSpec.IPAllocationPolicy.StackType {
			configCopy.Spec.IPAllocationPolicy.StackType = upstreamSpec.IPAllocationPolicy.StackType
			updateConfig = true
		}
		if configCopy.Spec.IPAllocationPolicy.IPv6AccessType != upstreamSpec.IPAllocationPolicy.IPv6AccessType {
			configCopy.Spec.IPAllocationPolicy.IPv6AccessType = upstreamSpec.IPAllocationPolicy.IPv6AccessType
			updateConfig = true
		}
	}
	if updateConfig {
		logrus.Infof("Syncing Dual-Stack/Dataplane configuration for cluster [%s]", config.Spec.ClusterName)
		return h.gkeCC.Update(configCopy)
	}
	changed, err := gke.UpdateMasterKubernetesVersion(h.gkeClientCtx, h.gkeClient, config, upstreamSpec)
	if err != nil {
		return config, err
	}
	if changed == gke.Changed {
		return h.enqueueUpdate(config)
	}

	changed, err = gke.UpdateClusterAddons(h.gkeClientCtx, h.gkeClient, config, upstreamSpec)
	if err != nil {
		return config, err
	}
	if changed == gke.Retry {
		h.gkeEnqueueAfter(config.Namespace, config.Name, wait*time.Second)
		return config, nil
	}
	if changed == gke.Changed {
		return h.enqueueUpdate(config)
	}

	changed, err = gke.UpdateMasterAuthorizedNetworks(h.gkeClientCtx, h.gkeClient, config, upstreamSpec)
	if err != nil {
		return config, err
	}
	if changed == gke.Changed {
		return h.enqueueUpdate(config)
	}

	changed, err = gke.UpdateLoggingMonitoringService(h.gkeClientCtx, h.gkeClient, config, upstreamSpec)
	if err != nil {
		return config, err
	}
	if changed == gke.Changed {
		return h.enqueueUpdate(config)
	}

	changed, err = gke.UpdateNetworkPolicyEnabled(h.gkeClientCtx, h.gkeClient, config, upstreamSpec)
	if err != nil {
		return config, err
	}
	if changed == gke.Changed {
		return h.enqueueUpdate(config)
	}

	changed, err = gke.UpdateLocations(h.gkeClientCtx, h.gkeClient, config, upstreamSpec)
	if err != nil {
		return config, err
	}
	if changed == gke.Changed {
		return h.enqueueUpdate(config)
	}

	changed, err = gke.UpdateMaintenanceWindow(h.gkeClientCtx, h.gkeClient, config, upstreamSpec)
	if err != nil {
		return config, err
	}
	if changed == gke.Changed {
		return h.enqueueUpdate(config)
	}

	changed, err = gke.UpdateLabels(h.gkeClientCtx, h.gkeClient, config, upstreamSpec)
	if err != nil {
		return config, err
	}
	if changed == gke.Changed || changed == gke.Retry {
		return h.enqueueUpdate(config)
	}

	if config.Spec.NodePools != nil && (config.Spec.AutopilotConfig == nil || !config.Spec.AutopilotConfig.Enabled) {
		downstreamNodePools, err := buildNodePoolMap(config.Spec.NodePools, config.Name)
		if err != nil {
			return config, err
		}

		upstreamNodePools, _ := buildNodePoolMap(upstreamSpec.NodePools, config.Name)
		nodePoolsNeedUpdate := false
		for npName, np := range downstreamNodePools {
			upstreamNodePool, ok := upstreamNodePools[npName]
			if ok {
				// There is a matching nodepool in the cluster already, so update it if needed
				changed, err = gke.UpdateNodePoolKubernetesVersionOrImageType(h.gkeClientCtx, h.gkeClient, np, config, upstreamNodePool)
				if err != nil {
					return config, err
				}
				if changed == gke.Changed || changed == gke.Retry {
					nodePoolsNeedUpdate = true
					// cannot make further updates while an operation is pending,
					// further updates will be retried if needed on the next reconcile loop
					continue
				}

				changed, err = gke.UpdateNodePoolSize(h.gkeClientCtx, h.gkeClient, np, config, upstreamNodePool)
				if err != nil {
					return config, err
				}
				if changed == gke.Changed || changed == gke.Retry {
					nodePoolsNeedUpdate = true
					// cannot make further updates while an operation is pending,
					// further updates will be retried if needed on the next reconcile loop
					continue
				}

				changed, err = gke.UpdateNodePoolAutoscaling(h.gkeClientCtx, h.gkeClient, np, config, upstreamNodePool)
				if err != nil {
					return config, err
				}
				if changed == gke.Changed || changed == gke.Retry {
					nodePoolsNeedUpdate = true
					// cannot make further updates while an operation is pending,
					// further updates will be retried if needed on the next reconcile loop
					continue
				}

				changed, err = gke.UpdateNodePoolManagement(h.gkeClientCtx, h.gkeClient, np, config, upstreamNodePool)
				if err != nil {
					return config, err
				}
				if changed == gke.Changed || changed == gke.Retry {
					nodePoolsNeedUpdate = true
					// cannot make further updates while an operation is pending,
					// further updates will be retried if needed on the next reconcile loop
					continue
				}

				changed, err = gke.UpdateNodePoolConfig(h.gkeClientCtx, h.gkeClient, np, config, upstreamNodePool)
				if err != nil {
					return config, err
				}
				if changed == gke.Changed || changed == gke.Retry {
					nodePoolsNeedUpdate = true
					// cannot make further updates while an operation is pending,
					// further updates will be retried if needed on the next reconcile loop
					continue
				}
			} else {
				// There is no nodepool with this name yet, create it
				logrus.Infof("Adding node pool [%s] to cluster [%s (id: %s)]", *np.Name, config.Spec.ClusterName, config.Name)
				if changed, err = gke.CreateNodePool(h.gkeClientCtx, h.gkeClient, config, np); err != nil {
					return config, err
				}
				if changed == gke.Changed || changed == gke.Retry {
					nodePoolsNeedUpdate = true
				}
			}
		}

		for npName := range upstreamNodePools {
			if _, ok := downstreamNodePools[npName]; !ok {
				logrus.Infof("Removing node pool [%s] from cluster [%s (id: %s)]", npName, config.Spec.ClusterName, config.Name)
				if changed, err = gke.RemoveNodePool(h.gkeClientCtx, h.gkeClient, config, npName); err != nil {
					return config, err
				}
				if changed == gke.Changed || changed == gke.Retry {
					nodePoolsNeedUpdate = true
				}
			}
		}
		if nodePoolsNeedUpdate {
			return h.enqueueUpdate(config)
		}
	}

	// no new updates, set to active
	if config.Status.Phase != gkeConfigActivePhase {
		logrus.Infof("Cluster [%s (id: %s)] finished updating", config.Spec.ClusterName, config.Name)
		config = config.DeepCopy()
		config.Status.Phase = gkeConfigActivePhase
		return h.gkeCC.UpdateStatus(config)
	}

	return config, nil
}

func (h *Handler) waitForCreationComplete(config *gkev1.GKEClusterConfig) (*gkev1.GKEClusterConfig, error) {
	cluster, err := gke.GetCluster(h.gkeClientCtx, h.gkeClient, &config.Spec)
	if err != nil {
		return config, err
	}
	if cluster.Status == ClusterStatusError {
		return config, fmt.Errorf("creation failed for cluster [%s (id: %s)]", config.Spec.ClusterName, config.Name)
	}
	if cluster.Status == ClusterStatusRunning {
		if err := h.createCASecret(config, cluster); err != nil {
			return config, err
		}
		logrus.Infof("Cluster [%s (id: %s)] is running", config.Spec.ClusterName, config.Name)
		config = config.DeepCopy()
		config.Status.Phase = gkeConfigActivePhase
		return h.gkeCC.UpdateStatus(config)
	}
	logrus.Infof("Waiting for cluster [%s (id: %s)] to finish creating", config.Spec.ClusterName, config.Name)
	h.gkeEnqueueAfter(config.Namespace, config.Name, wait*time.Second)

	return config, nil
}

func (h *Handler) buildUpstreamClusterState(cluster *gkeapi.Cluster) (*gkev1.GKEClusterConfigSpec, error) {
	newSpec := &gkev1.GKEClusterConfigSpec{
		KubernetesVersion:     &cluster.CurrentMasterVersion,
		EnableKubernetesAlpha: &cluster.EnableKubernetesAlpha,
		ClusterAddons:         &gkev1.GKEClusterAddons{},
		ClusterIpv4CidrBlock:  &cluster.ClusterIpv4Cidr,
		LoggingService:        &cluster.LoggingService,
		MonitoringService:     &cluster.MonitoringService,
		Network:               &cluster.Network,
		Subnetwork:            &cluster.Subnetwork,
		PrivateClusterConfig:  &gkev1.GKEPrivateClusterConfig{},
		IPAllocationPolicy:    &gkev1.GKEIPAllocationPolicy{},
		MasterAuthorizedNetworksConfig: &gkev1.GKEMasterAuthorizedNetworksConfig{
			Enabled: false,
		},
		Locations: cluster.Locations,
	}
	if cluster.ResourceLabels == nil {
		newSpec.Labels = make(map[string]string, 0)
	} else {
		newSpec.Labels = cluster.ResourceLabels
	}

	networkPolicyEnabled := false
	if cluster.NetworkPolicy != nil && cluster.NetworkPolicy.Enabled {
		networkPolicyEnabled = true
	}
	newSpec.NetworkPolicyEnabled = &networkPolicyEnabled

	enableDataplaneV2 := false
	if cluster.NetworkConfig != nil && cluster.NetworkConfig.DatapathProvider == "ADVANCED_DATAPATH" {
		enableDataplaneV2 = true
	}
	newSpec.EnableDataplaneV2 = &enableDataplaneV2

	if cluster.PrivateClusterConfig != nil {
		newSpec.PrivateClusterConfig.EnablePrivateEndpoint = cluster.PrivateClusterConfig.EnablePrivateEndpoint
		newSpec.PrivateClusterConfig.EnablePrivateNodes = cluster.PrivateClusterConfig.EnablePrivateNodes
		newSpec.PrivateClusterConfig.MasterIpv4CidrBlock = cluster.PrivateClusterConfig.MasterIpv4CidrBlock
	} else {
		newSpec.PrivateClusterConfig.EnablePrivateEndpoint = false
		newSpec.PrivateClusterConfig.EnablePrivateNodes = false
	}

	// build cluster addons
	if cluster.AddonsConfig != nil {
		lb := true
		if cluster.AddonsConfig.HttpLoadBalancing != nil {
			lb = !cluster.AddonsConfig.HttpLoadBalancing.Disabled
		}
		newSpec.ClusterAddons.HTTPLoadBalancing = lb
		hpa := true
		if cluster.AddonsConfig.HorizontalPodAutoscaling != nil {
			hpa = !cluster.AddonsConfig.HorizontalPodAutoscaling.Disabled
		}
		newSpec.ClusterAddons.HorizontalPodAutoscaling = hpa
		npc := true
		if cluster.AddonsConfig.NetworkPolicyConfig != nil {
			npc = !cluster.AddonsConfig.NetworkPolicyConfig.Disabled
		}
		newSpec.ClusterAddons.NetworkPolicyConfig = npc
	}

	if cluster.IpAllocationPolicy != nil {
		newSpec.IPAllocationPolicy.ClusterIpv4CidrBlock = cluster.IpAllocationPolicy.ClusterIpv4CidrBlock
		newSpec.IPAllocationPolicy.ClusterSecondaryRangeName = cluster.IpAllocationPolicy.ClusterSecondaryRangeName
		newSpec.IPAllocationPolicy.CreateSubnetwork = cluster.IpAllocationPolicy.CreateSubnetwork
		newSpec.IPAllocationPolicy.NodeIpv4CidrBlock = cluster.IpAllocationPolicy.NodeIpv4CidrBlock
		newSpec.IPAllocationPolicy.ServicesIpv4CidrBlock = cluster.IpAllocationPolicy.ServicesIpv4CidrBlock
		newSpec.IPAllocationPolicy.ServicesSecondaryRangeName = cluster.IpAllocationPolicy.ServicesSecondaryRangeName
		newSpec.IPAllocationPolicy.SubnetworkName = cluster.IpAllocationPolicy.SubnetworkName
		newSpec.IPAllocationPolicy.UseIPAliases = cluster.IpAllocationPolicy.UseIpAliases
		newSpec.IPAllocationPolicy.StackType = cluster.IpAllocationPolicy.StackType
		newSpec.IPAllocationPolicy.IPv6AccessType = cluster.IpAllocationPolicy.Ipv6AccessType
	}

	if cluster.MasterAuthorizedNetworksConfig != nil && cluster.MasterAuthorizedNetworksConfig.Enabled {
		newSpec.MasterAuthorizedNetworksConfig.Enabled = cluster.MasterAuthorizedNetworksConfig.Enabled
		for _, b := range cluster.MasterAuthorizedNetworksConfig.CidrBlocks {
			block := &gkev1.GKECidrBlock{
				CidrBlock:   b.CidrBlock,
				DisplayName: b.DisplayName,
			}
			newSpec.MasterAuthorizedNetworksConfig.CidrBlocks = append(newSpec.MasterAuthorizedNetworksConfig.CidrBlocks, block)
		}
	}

	window := ""
	newSpec.MaintenanceWindow = &window
	if cluster.MaintenancePolicy != nil && cluster.MaintenancePolicy.Window != nil && cluster.MaintenancePolicy.Window.DailyMaintenanceWindow != nil {
		newSpec.MaintenanceWindow = &cluster.MaintenancePolicy.Window.DailyMaintenanceWindow.StartTime
	}

	if cluster.Autopilot != nil && cluster.Autopilot.Enabled {
		newSpec.AutopilotConfig = &gkev1.GKEAutopilotConfig{
			Enabled: cluster.Autopilot.Enabled,
		}
	}

	// build node groups
	newSpec.NodePools = make([]gkev1.GKENodePoolConfig, 0, len(cluster.NodePools))

	for _, np := range cluster.NodePools {
		if np.Status == NodePoolStatusStopping {
			continue
		}
		np := np
		newNP := gkev1.GKENodePoolConfig{
			Name:             &np.Name,
			Version:          &np.Version,
			InitialNodeCount: &np.InitialNodeCount,
		}

		if np.Config != nil {
			newNP.Config = &gkev1.GKENodeConfig{
				BootDiskKmsKey: np.Config.BootDiskKmsKey,
				DiskSizeGb:     np.Config.DiskSizeGb,
				DiskType:       np.Config.DiskType,
				ImageType:      np.Config.ImageType,
				Labels:         np.Config.Labels,
				LocalSsdCount:  np.Config.LocalSsdCount,
				MachineType:    np.Config.MachineType,
				Preemptible:    np.Config.Preemptible,
				Tags:           np.Config.Tags,
				ServiceAccount: np.Config.ServiceAccount,
			}

			newNP.Config.Taints = make([]gkev1.GKENodeTaintConfig, 0, len(np.Config.Taints))
			for _, t := range np.Config.Taints {
				newNP.Config.Taints = append(newNP.Config.Taints, gkev1.GKENodeTaintConfig{
					Effect: t.Effect,
					Key:    t.Key,
					Value:  t.Value,
				})
			}
		}

		if np.Autoscaling != nil {
			newNP.Autoscaling = &gkev1.GKENodePoolAutoscaling{
				Enabled:      np.Autoscaling.Enabled,
				MaxNodeCount: np.Autoscaling.MaxNodeCount,
				MinNodeCount: np.Autoscaling.MinNodeCount,
			}
		}

		if np.Management != nil {
			newNP.Management = &gkev1.GKENodePoolManagement{
				AutoRepair:  np.Management.AutoRepair,
				AutoUpgrade: np.Management.AutoUpgrade,
			}
		}

		if np.MaxPodsConstraint != nil {
			newNP.MaxPodsConstraint = &np.MaxPodsConstraint.MaxPodsPerNode
		}
		newSpec.NodePools = append(newSpec.NodePools, newNP)
	}

	return newSpec, nil
}

// createCASecret creates a secret containing a CA and endpoint for use in generating a kubeconfig file.
func (h *Handler) createCASecret(config *gkev1.GKEClusterConfig, cluster *gkeapi.Cluster) error {
	var err error

	if cluster.Endpoint == "" {
		return fmt.Errorf("cluster [%s (id: %s)] has no endpoint", config.Spec.ClusterName, config.Name)
	}
	endpoint := cluster.Endpoint

	if cluster.MasterAuth == nil || cluster.MasterAuth.ClusterCaCertificate == "" {
		return fmt.Errorf("cluster [%s (id: %s)] has no CA", config.Spec.ClusterName, config.Name)
	}
	ca := []byte(cluster.MasterAuth.ClusterCaCertificate)

	_, err = h.secrets.Create(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      config.Name,
				Namespace: config.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: gkev1.SchemeGroupVersion.String(),
						Kind:       gkeClusterConfigKind,
						UID:        config.UID,
						Name:       config.Name,
					},
				},
			},
			Data: map[string][]byte{
				"endpoint": []byte(endpoint),
				"ca":       ca,
			},
		})
	if errors.IsAlreadyExists(err) {
		logrus.Debugf("CA secret [%s] already exists, ignoring", config.Name)
		return nil
	}
	return err
}

func buildNodePoolMap(nodePools []gkev1.GKENodePoolConfig, clusterName string) (map[string]*gkev1.GKENodePoolConfig, error) {
	ret := make(map[string]*gkev1.GKENodePoolConfig, len(nodePools))
	for i := range nodePools {
		if nodePools[i].Name != nil {
			if _, ok := ret[*nodePools[i].Name]; ok {
				return nil, fmt.Errorf("cluster [%s] cannot have multiple nodepools with name %s", clusterName, *nodePools[i].Name)
			}
			ret[*nodePools[i].Name] = &nodePools[i]
		}
	}
	return ret, nil
}
