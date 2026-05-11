package gke

import (
	"context"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	gkev1 "github.com/rancher/gke-operator/pkg/apis/gke.cattle.io/v1"
	"github.com/rancher/gke-operator/pkg/gke/services"
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
func RemoveCluster(ctx context.Context, gkeClient services.GKEClusterService, config *gkev1.GKEClusterConfig) error {
	clusterRRN := ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName)

	return wait.ExponentialBackoff(backoff, func() (bool, error) {
		_, err := gkeClient.ClusterDelete(ctx, clusterRRN)

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
func RemoveNodePool(ctx context.Context, gkeClient services.GKEClusterService, config *gkev1.GKEClusterConfig, nodePoolName string) (Status, error) {
	_, err := gkeClient.NodePoolDelete(ctx,
		NodePoolRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName, nodePoolName))
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
