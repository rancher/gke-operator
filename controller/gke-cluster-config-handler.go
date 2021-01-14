package controller

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	gkev1 "github.com/rancher/gke-operator/pkg/apis/gke.cattle.io/v1"
	v12 "github.com/rancher/gke-operator/pkg/generated/controllers/gke.cattle.io/v1"
	"github.com/rancher/rke/log"
	wranglerv1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gkeapi "google.golang.org/api/container/v1"
	"google.golang.org/api/option"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	controllerName           = "gke-controller"
	controllerRemoveName     = "gke-controller-remove"
	gkeConfigCreatingPhase   = "creating"
	gkeConfigNotCreatedPhase = ""
	gkeConfigActivePhase     = "active"
	gkeConfigUpdatingPhase   = "updating"
	gkeConfigImportingPhase  = "importing"
	allOpen                  = "0.0.0.0/0"
	gkeClusterConfigKind     = "GKEClusterConfig"
	runningStatus            = "RUNNING"
	none                     = "none"
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
		fmt.Errorf("not implemented")
	case gkeConfigNotCreatedPhase:
		return h.create(config)
	case gkeConfigCreatingPhase:
		return h.waitForCreationComplete(config)
	case gkeConfigActivePhase:
		return h.checkAndUpdate(config)
	case gkeConfigUpdatingPhase:
		fmt.Errorf("not implemented")
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
		if err != nil {
			if !strings.Contains(err.Error(), "currently has update") {
				// the update is valid in that the controller should retry but there is
				// no actionable resolution as far as a user is concerned. An update
				// that has either been initiated by gke-operator or another source is
				// already in progress. It is possible an update is not being immediately
				// reflected in upstream cluster state. The config object will reenter the
				// controller and then the controller will wait for the update to finish.
				message = err.Error()
			}
		}

		if config.Status.FailureMessage == message {
			return config, err
		}

		if message != "" {
			config = config.DeepCopy()
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

	//TODO: delete cluster
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc, err := getServiceClient(ctx, config.Spec.CredentialContent)
	if err != nil {
		return config, err
	}

	logrus.Debugf("Removing cluster %v from project %v, region/zone %v", config.Spec.ClusterName, config.Spec.ProjectID, config.Spec.Region)
	operation, err := waitClusterRemoveExp(ctx, svc, config)

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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc, err := getServiceClient(ctx, config.Spec.CredentialContent)
	if err != nil {
		return config, err
	}

	operation, err := svc.Projects.Locations.Clusters.Create(
		locationRRN(config.Spec.ProjectID, config.Spec.Region),
		generateClusterCreateRequest(config)).Context(ctx).Do()

	if err != nil && !strings.Contains(err.Error(), "alreadyExists") {
		return config, err
	}

	if err == nil {
		logrus.Debugf("Cluster %s create is called for project %s and region/zone %s. Status Code %v",
			config.Spec.ClusterName, config.Spec.ProjectID, config.Spec.Region, operation.HTTPStatusCode)
	}

	config = config.DeepCopy()
	config.Status.Phase = gkeConfigCreatingPhase
	return h.gkeCC.UpdateStatus(config)
}

func (h *Handler) checkAndUpdate(config *gkev1.GKEClusterConfig) (*gkev1.GKEClusterConfig, error) {

	//TODO: implement -- currently a noop.
	return config, nil
}

func (h *Handler) waitForCreationComplete(config *gkev1.GKEClusterConfig) (*gkev1.GKEClusterConfig, error) {
	lastMsg := ""
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for {
		svc, err := getServiceClient(ctx, config.Spec.CredentialContent)
		if err != nil {
			return config, err
		}
		cluster, err := svc.Projects.Locations.Clusters.Get(clusterRRN(config.Spec.ProjectID, config.Spec.Region, config.Spec.ClusterName)).Context(ctx).Do()
		if err != nil {
			return config, err
		}
		if cluster.Status == runningStatus {
			log.Infof(ctx, "Cluster %v is running", config.Spec.ClusterName)
			config = config.DeepCopy()
			config.Status.Phase = gkeConfigActivePhase
			return h.gkeCC.UpdateStatus(config)
		}
		if cluster.Status != lastMsg {
			log.Infof(ctx, "%v cluster %v......", strings.ToLower(cluster.Status), config.Spec.ClusterName)
			lastMsg = cluster.Status
		}
		time.Sleep(time.Second * 5)
	}

}

func waitClusterRemoveExp(ctx context.Context, svc *gkeapi.Service, config *gkev1.GKEClusterConfig) (*gkeapi.Operation, error) {
	var operation *gkeapi.Operation
	var err error

	for i := 1; i < 12; i++ {
		time.Sleep(time.Duration(i*i) * time.Second)
		operation, err = svc.Projects.Locations.Clusters.Delete(clusterRRN(config.Spec.ProjectID, config.Spec.Region, config.Spec.ClusterName)).Context(ctx).Do()
		if err == nil {
			return operation, nil
		} else if !strings.Contains(err.Error(), "Please wait and try again once it is done") {
			break
		}
	}
	return operation, err
}

func getClientset(cluster *gkeapi.Cluster, ts oauth2.TokenSource) (kubernetes.Interface, error) {
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

func generateClusterCreateRequest(config *gkev1.GKEClusterConfig) *gkeapi.CreateClusterRequest {
	request := &gkeapi.CreateClusterRequest{
		Cluster: &gkeapi.Cluster{
			NodePools: []*gkeapi.NodePool{},
		},
	}
	request.Cluster.Name = config.Spec.ClusterName
	request.Cluster.InitialClusterVersion = *config.Spec.KubernetesVersion
	request.Cluster.Description = config.Spec.Description
	request.Cluster.EnableKubernetesAlpha = config.Spec.EnableAlphaFeature
	request.Cluster.ClusterIpv4Cidr = config.Spec.ClusterIpv4Cidr
	request.Cluster.EnableKubernetesAlpha = config.Spec.EnableAlphaFeature

	disableHTTPLoadBalancing := config.Spec.ClusterAddons.HTTPLoadBalancing != nil && !*config.Spec.ClusterAddons.HTTPLoadBalancing
	disableHorizontalPodAutoscaling := config.Spec.ClusterAddons.HorizontalPodAutoscaling != nil && !*config.Spec.ClusterAddons.HorizontalPodAutoscaling
	disableNetworkPolicyConfig := config.Spec.ClusterAddons.NetworkPolicyConfig != nil && !*config.Spec.ClusterAddons.NetworkPolicyConfig
	disableKubernetesDashboard := config.Spec.ClusterAddons.KubernetesDashboard != nil && !*config.Spec.ClusterAddons.KubernetesDashboard

	request.Cluster.AddonsConfig = &gkeapi.AddonsConfig{
		HttpLoadBalancing:        &gkeapi.HttpLoadBalancing{Disabled: !disableHTTPLoadBalancing},
		HorizontalPodAutoscaling: &gkeapi.HorizontalPodAutoscaling{Disabled: disableHorizontalPodAutoscaling},
		KubernetesDashboard:      &gkeapi.KubernetesDashboard{Disabled: disableKubernetesDashboard},
		NetworkPolicyConfig:      &gkeapi.NetworkPolicyConfig{Disabled: disableNetworkPolicyConfig},
	}

	request.Cluster.InitialNodeCount = 1

	return request
}

func getServiceClient(ctx context.Context, credential string) (*gkeapi.Service, error) {
	ts, err := GetTokenSource(ctx, credential)
	if err != nil {
		return nil, err
	}
	return getServiceClientWithTokenSource(ctx, ts)
}

func getServiceClientWithTokenSource(ctx context.Context, ts oauth2.TokenSource) (*gkeapi.Service, error) {
	client := oauth2.NewClient(ctx, ts)
	return gkeapi.NewService(ctx, option.WithHTTPClient(client))
}

func GetTokenSource(ctx context.Context, credential string) (oauth2.TokenSource, error) {
	ts, err := google.CredentialsFromJSON(ctx, []byte(credential), gkeapi.CloudPlatformScope)
	if err != nil {
		return nil, err
	}
	return ts.TokenSource, nil
}

// locationRRN returns a Relative Resource Name representing a location. This
// RRN can either represent a Region or a Zone. It can be used as the parent
// attribute during cluster creation to create a zonal or regional cluster, or
// be used to generate more specific RRNs like an RRN representing a cluster.
//
// https://cloud.google.com/apis/design/resource_names#relative_resource_name
func locationRRN(projectID, location string) string {
	return fmt.Sprintf("projects/%s/locations/%s", projectID, location)
}

// clusterRRN returns an Relative Resource Name of a cluster in the specified
// region or zone
func clusterRRN(projectID, location, clusterName string) string {
	return fmt.Sprintf("%s/clusters/%s", locationRRN(projectID, location), clusterName)
}

// nodePoolRRN returns a Relative Resource Name of a node pool in a cluster in the
// region or zone for the specified project
func nodePoolRRN(projectID, location, clusterName, nodePool string) string {
	return fmt.Sprintf("%s/nodePools/%s", clusterRRN(projectID, location, clusterName), nodePool)
}
