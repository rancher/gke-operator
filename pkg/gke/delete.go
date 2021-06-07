package gke

import (
	"context"
	"strings"
	"time"

	gkev1 "github.com/rancher/gke-operator/pkg/apis/gke.cattle.io/v1"
	gkeapi "google.golang.org/api/container/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	waitSec      = 30
	backoffSteps = 12
)

var backoff = wait.Backoff{
	Duration: waitSec * time.Second,
	Steps:    backoffSteps,
}

// RemoveCluster attempts to delete a cluster and retries the delete request if the cluster is busy.
func RemoveCluster(ctx context.Context, client *gkeapi.Service, config *gkev1.GKEClusterConfig) error {
	return wait.ExponentialBackoff(backoff, func() (bool, error) {
		_, err := client.Projects.
			Locations.
			Clusters.
			Delete(ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName)).
			Context(ctx).
			Do()
		if err != nil && strings.Contains(err.Error(), errWait) {
			return false, nil
		}
		if err != nil && strings.Contains(err.Error(), errNotFound) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		return true, nil
	})
}

// RemoveNodePool deletes a node pool
func RemoveNodePool(ctx context.Context, client *gkeapi.Service, config *gkev1.GKEClusterConfig, nodePoolName string) (Status, error) {
	_, err := client.Projects.
		Locations.
		Clusters.
		NodePools.
		Delete(NodePoolRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName, nodePoolName)).
		Context(ctx).
		Do()
	if err != nil && strings.Contains(err.Error(), errWait) {
		return Retry, nil
	}
	if err != nil && strings.Contains(err.Error(), errNotFound) {
		return NotChanged, nil
	}
	if err != nil {
		return NotChanged, err
	}
	return Changed, nil
}
