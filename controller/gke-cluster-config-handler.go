package controller

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/rancher/gke-operator/internal/gke"
	"github.com/rancher/gke-operator/internal/utils"
	gkev1 "github.com/rancher/gke-operator/pkg/apis/gke.cattle.io/v1"
	v12 "github.com/rancher/gke-operator/pkg/generated/controllers/gke.cattle.io/v1"
	wranglerv1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	gkeapi "google.golang.org/api/container/v1"
	corev1 "k8s.io/api/core/v1"
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cred, err := getSecret(ctx, h.secretsCache, &config.Spec)
	if err != nil {
		return config, err
	}
	client, err := gke.GetGKEClient(ctx, cred)
	if err != nil {
		return config, err
	}

	logrus.Infof("removing cluster %v from project %v, region/zone %v", config.Spec.ClusterName, config.Spec.ProjectID, config.Spec.Region)
	if err := gke.RemoveCluster(ctx, client, config); err != nil {
		logrus.Debugf("error deleting cluster %s: %v", config.Spec.ClusterName, err)
		return config, err
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

	cred, err := getSecret(ctx, h.secretsCache, &config.Spec)
	if err != nil {
		return config, err
	}
	client, err := gke.GetGKEClient(ctx, cred)
	if err != nil {
		return config, err
	}

	if err = gke.Create(ctx, client, config); err != nil {
		return config, err
	}

	config = config.DeepCopy()
	config.Status.Phase = gkeConfigCreatingPhase
	config, err = h.gkeCC.UpdateStatus(config)
	return config, err
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
	client, err := gke.GetGKEClient(ctx, cred)
	if err != nil {
		return config, err
	}

	changed, err := gke.UpdateMasterKubernetesVersion(ctx, client, config, upstreamSpec)
	if err != nil {
		return config, err
	}
	if changed == gke.Changed {
		return h.enqueueUpdate(config)
	}

	changed, err = gke.UpdateClusterAddons(ctx, client, config, upstreamSpec)
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

	changed, err = gke.UpdateMasterAuthorizedNetworks(ctx, client, config, upstreamSpec)
	if err != nil {
		return config, err
	}
	if changed == gke.Changed {
		return h.enqueueUpdate(config)
	}

	changed, err = gke.UpdateLoggingMonitoringService(ctx, client, config, upstreamSpec)
	if err != nil {
		return config, err
	}
	if changed == gke.Changed {
		return h.enqueueUpdate(config)
	}

	changed, err = gke.UpdateNetworkPolicyEnabled(ctx, client, config, upstreamSpec)
	if err != nil {
		return config, err
	}
	if changed == gke.Changed {
		return h.enqueueUpdate(config)
	}

	if config.Spec.NodePools != nil {
		upstreamNodePools := buildNodePoolMap(upstreamSpec.NodePools)
		nodePoolsNeedUpdate := false
		for _, np := range config.Spec.NodePools {
			upstreamNodePool, ok := upstreamNodePools[*np.Name]
			if ok {
				// There is a matching nodepool in the cluster already, so update it if needed
				changed, err = gke.UpdateNodePoolKubernetesVersionOrImageType(ctx, client, &np, config, upstreamNodePool)
				if err != nil {
					return config, err
				}
				if changed == gke.Changed || changed == gke.Retry {
					nodePoolsNeedUpdate = true
					// cannot make further updates while an operation is pending,
					// further updates will be retried if needed on the next reconcile loop
					continue
				}

				changed, err = gke.UpdateNodePoolSize(ctx, client, &np, config, upstreamNodePool)
				if err != nil {
					return config, err
				}
				if changed == gke.Changed || changed == gke.Retry {
					nodePoolsNeedUpdate = true
					// cannot make further updates while an operation is pending,
					// further updates will be retried if needed on the next reconcile loop
					continue
				}

				changed, err = gke.UpdateNodePoolAutoscaling(ctx, client, &np, config, upstreamNodePool)
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
				if changed, err = gke.CreateNodePool(ctx, client, config, &np); err != nil {
					return config, err
				}
				if changed == gke.Changed || changed == gke.Retry {
					nodePoolsNeedUpdate = true
				}
			}
		}
		downstreamNodePools := buildNodePoolMap(config.Spec.NodePools)
		for _, np := range upstreamSpec.NodePools {
			if _, ok := downstreamNodePools[*np.Name]; !ok {
				logrus.Infof("removing node pool [%s] from cluster [%s]", *np.Name, config.Name)
				if changed, err = gke.RemoveNodePool(ctx, client, config, *np.Name); err != nil {
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

func getSecret(ctx context.Context, secretsCache wranglerv1.SecretCache, configSpec *gkev1.GKEClusterConfigSpec) (string, error) {
	ns, id := parseCredential(configSpec.CredentialContent)
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

func buildNodePoolMap(nodePools []gkev1.NodePoolConfig) map[string]*gkev1.NodePoolConfig {
	ret := make(map[string]*gkev1.NodePoolConfig)
	for i := range nodePools {
		if nodePools[i].Name != nil {
			ret[*nodePools[i].Name] = &nodePools[i]
		}
	}
	return ret
}

func GetCluster(ctx context.Context, secretsCache wranglerv1.SecretCache, configSpec *gkev1.GKEClusterConfigSpec) (*gkeapi.Cluster, error) {
	cred, err := getSecret(ctx, secretsCache, configSpec)
	if err != nil {
		return nil, err
	}
	client, err := gke.GetGKEClient(ctx, cred)
	if err != nil {
		return nil, err
	}
	cluster, err := client.Projects.
		Locations.
		Clusters.
		Get(gke.ClusterRRN(configSpec.ProjectID, configSpec.Region, configSpec.ClusterName)).
		Context(ctx).
		Do()
	if err != nil {
		return nil, err
	}
	return cluster, nil
}

func BuildUpstreamClusterState(cluster *gkeapi.Cluster) (*gkev1.GKEClusterConfigSpec, error) {
	newSpec := &gkev1.GKEClusterConfigSpec{
		KubernetesVersion:    &cluster.CurrentMasterVersion,
		EnableAlphaFeature:   &cluster.EnableKubernetesAlpha,
		ClusterAddons:        &gkev1.ClusterAddons{},
		ClusterIpv4CidrBlock: &cluster.ClusterIpv4Cidr,
		LoggingService:       &cluster.LoggingService,
		MonitoringService:    &cluster.MonitoringService,
		Network:              &cluster.Network,
		Subnetwork:           &cluster.Subnetwork,
		PrivateClusterConfig: &gkev1.PrivateClusterConfig{},
		IPAllocationPolicy:   &gkev1.IPAllocationPolicy{},
		MasterAuthorizedNetworksConfig: &gkev1.MasterAuthorizedNetworksConfig{
			Enabled: false,
		},
	}

	networkPolicyEnabled := false
	if cluster.NetworkPolicy != nil && cluster.NetworkPolicy.Enabled == true {
		networkPolicyEnabled = true
	}
	newSpec.NetworkPolicyEnabled = &networkPolicyEnabled

	if cluster.PrivateClusterConfig != nil {
		newSpec.PrivateClusterConfig.EnablePrivateEndpoint = cluster.PrivateClusterConfig.EnablePrivateNodes
		newSpec.PrivateClusterConfig.EnablePrivateNodes = cluster.PrivateClusterConfig.EnablePrivateNodes
		newSpec.PrivateClusterConfig.MasterIpv4CidrBlock = cluster.PrivateClusterConfig.MasterIpv4CidrBlock
		newSpec.PrivateClusterConfig.PrivateEndpoint = cluster.PrivateClusterConfig.PrivateEndpoint
		newSpec.PrivateClusterConfig.PublicEndpoint = cluster.PrivateClusterConfig.PublicEndpoint
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
			block := &gkev1.CidrBlock{
				CidrBlock:   b.CidrBlock,
				DisplayName: b.DisplayName,
			}
			newSpec.MasterAuthorizedNetworksConfig.CidrBlocks = append(newSpec.MasterAuthorizedNetworksConfig.CidrBlocks, block)
		}
	}

	// build node groups
	newSpec.NodePools = make([]gkev1.NodePoolConfig, 0, len(cluster.NodePools))

	for _, np := range cluster.NodePools {
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

// createCASecret creates a secret containing a CA and endpoint for use in generating a kubeconfig file.
func (h *Handler) createCASecret(config *gkev1.GKEClusterConfig, cluster *gkeapi.Cluster) error {
	var err error
	endpoint := cluster.Endpoint
	var ca []byte
	if cluster.MasterAuth != nil {
		// ClusterCaCertificate is base64-encoded, so it needs to be decoded
		// before being passed to secrets.Create which will reencode it
		encodedCA := cluster.MasterAuth.ClusterCaCertificate
		ca, err = base64.StdEncoding.DecodeString(encodedCA)
		if err != nil {
			return err
		}
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
	return err
}
