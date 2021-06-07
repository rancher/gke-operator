package gke

import (
	"fmt"
)

// Location returns the region or zone depending on which is not set to empty.
// Cluster creation validation should ensure that only one of region or zone is set, not both.
func Location(region, zone string) string {
	ret := region
	if zone != "" {
		ret = zone
	}
	return ret
}

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
