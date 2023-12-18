package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	gkev1 "github.com/rancher/gke-operator/pkg/apis/gke.cattle.io/v1"
	gkecontrollers "github.com/rancher/gke-operator/pkg/generated/controllers/gke.cattle.io/v1"
	"github.com/rancher/gke-operator/pkg/gke"
	wranglerv1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"

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
			logrus.Errorf("Error recording gkecc [%s] failure message: %s, original error: %s", config.Name, recordErr, err)
		}
		return config, err
	}
}

// importCluster returns an active cluster spec containing the given config's clusterName and region/zone
// and creates a Secret containing the cluster's CA and endpoint retrieved from the cluster object.
func (h *Handler) importCluster(config *gkev1.GKEClusterConfig) (*gkev1.GKEClusterConfig, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cluster, err := GetCluster(ctx, h.secretsCache, &config.Spec)
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
	if config.Spec.Imported {
		logrus.Infof("cluster [%s] is imported, will not delete GKE cluster", config.Name)
		return config, nil
	}
	if config.Status.Phase == gkeConfigNotCreatedPhase {
		// The most likely context here is that the cluster already existed in GKE, so we shouldn't delete it
		logrus.Warnf("cluster [%s] never advanced to creating status, will not delete GKE cluster", config.Name)
		return config, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cred, err := getSecret(ctx, h.secretsCache, &config.Spec)
	if err != nil {
		return config, err
	}
	gkeClient, err := gke.GetGKEClusterClient(ctx, cred)
	if err != nil {
		return config, err
	}

	logrus.Infof("removing cluster %v from project %v, region/zone %v", config.Spec.ClusterName, config.Spec.ProjectID, gke.Location(config.Spec.Region, config.Spec.Zone))
	if err := gke.RemoveCluster(ctx, gkeClient, config); err != nil {
		logrus.Debugf("error deleting cluster %s: %v", config.Spec.ClusterName, err)
		return config, err
	}

	return config, nil
}

func (h *Handler) create(config *gkev1.GKEClusterConfig) (*gkev1.GKEClusterConfig, error) {
	if config.Spec.Imported {
		logrus.Infof("importing cluster [%s]", config.Name)
		config = config.DeepCopy()
		config.Status.Phase = gkeConfigImportingPhase
		return h.gkeCC.UpdateStatus(config)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cred, err := getSecret(ctx, h.secretsCache, &config.Spec)
	if err != nil {
		return config, err
	}
	gkeClient, err := gke.GetGKEClusterClient(ctx, cred)
	if err != nil {
		return config, err
	}

	if err = gke.Create(ctx, gkeClient, config); err != nil {
		return config, err
	}

	config = config.DeepCopy()
	config.Status.Phase = gkeConfigCreatingPhase
	config, err = h.gkeCC.UpdateStatus(config)
	return config, err
}

func (h *Handler) checkAndUpdate(config *gkev1.GKEClusterConfig) (*gkev1.GKEClusterConfig, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cluster, err := GetCluster(ctx, h.secretsCache, &config.Spec)
	if err != nil {
		return config, err
	}

	if cluster.Status == ClusterStatusReconciling {
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
			logrus.Infof("waiting for cluster [%s] to update node pool [%s]", config.Name, np.Name)
			h.gkeEnqueueAfter(config.Namespace, config.Name, 30*time.Second)
			return config, nil
		}
	}

	upstreamSpec, err := BuildUpstreamClusterState(cluster)
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cred, err := getSecret(ctx, h.secretsCache, &config.Spec)
	if err != nil {
		return config, err
	}
	gkeClient, err := gke.GetGKEClusterClient(ctx, cred)
	if err != nil {
		return config, err
	}

	changed, err := gke.UpdateMasterKubernetesVersion(ctx, gkeClient, config, upstreamSpec)
	if err != nil {
		return config, err
	}
	if changed == gke.Changed {
		return h.enqueueUpdate(config)
	}

	changed, err = gke.UpdateClusterAddons(ctx, gkeClient, config, upstreamSpec)
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

	changed, err = gke.UpdateMasterAuthorizedNetworks(ctx, gkeClient, config, upstreamSpec)
	if err != nil {
		return config, err
	}
	if changed == gke.Changed {
		return h.enqueueUpdate(config)
	}

	changed, err = gke.UpdateLoggingMonitoringService(ctx, gkeClient, config, upstreamSpec)
	if err != nil {
		return config, err
	}
	if changed == gke.Changed {
		return h.enqueueUpdate(config)
	}

	changed, err = gke.UpdateNetworkPolicyEnabled(ctx, gkeClient, config, upstreamSpec)
	if err != nil {
		return config, err
	}
	if changed == gke.Changed {
		return h.enqueueUpdate(config)
	}

	changed, err = gke.UpdateLocations(ctx, gkeClient, config, upstreamSpec)
	if err != nil {
		return config, err
	}
	if changed == gke.Changed {
		return h.enqueueUpdate(config)
	}

	changed, err = gke.UpdateMaintenanceWindow(ctx, gkeClient, config, upstreamSpec)
	if err != nil {
		return config, err
	}
	if changed == gke.Changed {
		return h.enqueueUpdate(config)
	}

	changed, err = gke.UpdateLabels(ctx, gkeClient, config, upstreamSpec)
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
				changed, err = gke.UpdateNodePoolKubernetesVersionOrImageType(ctx, gkeClient, np, config, upstreamNodePool)
				if err != nil {
					return config, err
				}
				if changed == gke.Changed || changed == gke.Retry {
					nodePoolsNeedUpdate = true
					// cannot make further updates while an operation is pending,
					// further updates will be retried if needed on the next reconcile loop
					continue
				}

				changed, err = gke.UpdateNodePoolSize(ctx, gkeClient, np, config, upstreamNodePool)
				if err != nil {
					return config, err
				}
				if changed == gke.Changed || changed == gke.Retry {
					nodePoolsNeedUpdate = true
					// cannot make further updates while an operation is pending,
					// further updates will be retried if needed on the next reconcile loop
					continue
				}

				changed, err = gke.UpdateNodePoolAutoscaling(ctx, gkeClient, np, config, upstreamNodePool)
				if err != nil {
					return config, err
				}
				if changed == gke.Changed || changed == gke.Retry {
					nodePoolsNeedUpdate = true
					// cannot make further updates while an operation is pending,
					// further updates will be retried if needed on the next reconcile loop
					continue
				}

				changed, err = gke.UpdateNodePoolManagement(ctx, gkeClient, np, config, upstreamNodePool)
				if err != nil {
					return config, err
				}
				if changed == gke.Changed || changed == gke.Retry {
					nodePoolsNeedUpdate = true
					// cannot make further updates while an operation is pending,
					// further updates will be retried if needed on the next reconcile loop
					continue
				}

				changed, err = gke.UpdateNodePoolConfig(ctx, gkeClient, np, config, upstreamNodePool)
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
				logrus.Infof("adding node pool [%s] to cluster [%s]", *np.Name, config.Name)
				if changed, err = gke.CreateNodePool(ctx, gkeClient, config, np); err != nil {
					return config, err
				}
				if changed == gke.Changed || changed == gke.Retry {
					nodePoolsNeedUpdate = true
				}
			}
		}

		for npName := range upstreamNodePools {
			if _, ok := downstreamNodePools[npName]; !ok {
				logrus.Infof("removing node pool [%s] from cluster [%s]", npName, config.Name)
				if changed, err = gke.RemoveNodePool(ctx, gkeClient, config, npName); err != nil {
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

	cluster, err := GetCluster(ctx, h.secretsCache, &config.Spec)
	if err != nil {
		return config, err
	}
	if cluster.Status == ClusterStatusError {
		return config, fmt.Errorf("creation failed for cluster %v", config.Spec.ClusterName)
	}
	if cluster.Status == ClusterStatusRunning {
		if err := h.createCASecret(config, cluster); err != nil {
			return config, err
		}
		logrus.Infof("Cluster %v is running", config.Spec.ClusterName)
		config = config.DeepCopy()
		config.Status.Phase = gkeConfigActivePhase
		return h.gkeCC.UpdateStatus(config)
	}
	logrus.Infof("waiting for cluster [%s] to finish creating", config.Name)
	h.gkeEnqueueAfter(config.Namespace, config.Name, wait*time.Second)

	return config, nil
}

func getSecret(_ context.Context, secretsCache wranglerv1.SecretCache, configSpec *gkev1.GKEClusterConfigSpec) (string, error) {
	ns, id := parseCredential(configSpec.GoogleCredentialSecret)
	secret, err := secretsCache.Get(ns, id)
	if err != nil {
		return "", err
	}
	dataBytes, ok := secret.Data["googlecredentialConfig-authEncodedJson"]
	if !ok {
		return "", fmt.Errorf("could not read malformed cloud credential secret %s from namespace %s", id, ns)
	}
	return string(dataBytes), nil
}

func parseCredential(ref string) (namespace string, name string) {
	parts := strings.SplitN(ref, ":", 2)
	if len(parts) == 1 {
		return "", parts[0]
	}
	return parts[0], parts[1]
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

func GetCluster(ctx context.Context, secretsCache wranglerv1.SecretCache, configSpec *gkev1.GKEClusterConfigSpec) (*gkeapi.Cluster, error) {
	cred, err := getSecret(ctx, secretsCache, configSpec)
	if err != nil {
		return nil, err
	}
	gkeClient, err := gke.GetGKEClusterClient(ctx, cred)
	if err != nil {
		return nil, err
	}
	return gke.GetCluster(ctx, gkeClient, configSpec)
}

func BuildUpstreamClusterState(cluster *gkeapi.Cluster) (*gkev1.GKEClusterConfigSpec, error) {
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
				DiskSizeGb:    np.Config.DiskSizeGb,
				DiskType:      np.Config.DiskType,
				ImageType:     np.Config.ImageType,
				Labels:        np.Config.Labels,
				LocalSsdCount: np.Config.LocalSsdCount,
				MachineType:   np.Config.MachineType,
				Preemptible:   np.Config.Preemptible,
				Tags:          np.Config.Tags,
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
	endpoint := cluster.Endpoint
	var ca []byte
	if cluster.MasterAuth != nil {
		ca = []byte(cluster.MasterAuth.ClusterCaCertificate)
	}

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

func GetTokenSource(ctx context.Context, secretsCache wranglerv1.SecretCache, configSpec *gkev1.GKEClusterConfigSpec) (oauth2.TokenSource, error) {
	cred, err := getSecret(ctx, secretsCache, configSpec)
	if err != nil {
		return nil, fmt.Errorf("error getting secret: %w", err)
	}
	ts, err := gke.GetTokenSource(ctx, cred)
	if err != nil {
		return nil, fmt.Errorf("error getting oauth2 token: %w", err)
	}
	return ts, nil
}
