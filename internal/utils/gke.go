package utils

import (
	"fmt"
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
