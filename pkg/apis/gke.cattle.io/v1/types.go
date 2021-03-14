/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type GKEClusterConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GKEClusterConfigSpec   `json:"spec"`
	Status GKEClusterConfigStatus `json:"status"`
}

// GKEClusterConfigSpec is the spec for a GKEClusterConfig resource
type GKEClusterConfigSpec struct {
	Region                         string                          `json:"region" norman:"noupdate"`
	Zone                           string                          `json:"zone" norman:"noupdate"`
	Imported                       bool                            `json:"imported" norman:"noupdate"`
	Description                    string                          `json:"description"`
	EnableAlphaFeature             *bool                           `json:"enableAlphaFeature"`
	ClusterAddons                  *ClusterAddons                  `json:"clusterAddons"`
	ClusterIpv4CidrBlock           *string                         `json:"clusterIpv4Cidr"`
	ProjectID                      string                          `json:"projectID"`
	CredentialContent              string                          `json:"credentialContent"`
	ClusterName                    string                          `json:"clusterName"`
	KubernetesVersion              *string                         `json:"kubernetesVersion"`
	LoggingService                 *string                         `json:"loggingService"`
	MonitoringService              *string                         `json:"monitoringService"`
	NodePools                      []NodePoolConfig                `json:"nodePools"`
	Network                        *string                         `json:"network,omitempty"`
	Subnetwork                     *string                         `json:"subnetwork,omitempty"`
	NetworkPolicyEnabled           *bool                           `json:"networkPolicyEnabled,omitempty"`
	PrivateClusterConfig           *PrivateClusterConfig           `json:"privateClusterConfig,omitempty"`
	IPAllocationPolicy             *IPAllocationPolicy             `json:"ipAllocationPolicy,omitempty" norman:"noupdate"`
	MasterAuthorizedNetworksConfig *MasterAuthorizedNetworksConfig `json:"masterAuthorizedNetworks,omitempty" norman:"noupdate"`
}

type IPAllocationPolicy struct {
	ClusterIpv4CidrBlock       string `json:"clusterIpv4CidrBlock,omitempty"`
	ClusterSecondaryRangeName  string `json:"clusterSecondaryRangeName,omitempty"`
	CreateSubnetwork           bool   `json:"createSubnetwork,omitempty"`
	NodeIpv4CidrBlock          string `json:"nodeIpv4CidrBlock,omitempty"`
	ServicesIpv4CidrBlock      string `json:"servicesIpv4CidrBlock,omitempty"`
	ServicesSecondaryRangeName string `json:"servicesSecondaryRangeName,omitempty"`
	SubnetworkName             string `json:"subnetworkName,omitempty"`
	UseIPAliases               bool   `json:"useIpAliases,omitempty"`
}

type PrivateClusterConfig struct {
	EnablePrivateEndpoint *bool  `json:"enablePrivateEndpoint,omitempty"`
	EnablePrivateNodes    *bool  `json:"enablePrivateNodes,omitempty"`
	MasterIpv4CidrBlock   string `json:"masterIpv4CidrBlock,omitempty"`
	PrivateEndpoint       string `json:"privateEndpoint,omitempty"`
	PublicEndpoint        string `json:"publicEndpoint,omitempty"`
}

type GKEClusterConfigStatus struct {
	Phase          string `json:"phase"`
	FailureMessage string `json:"failureMessage"`
}

type ClusterAddons struct {
	HTTPLoadBalancing        bool `json:"httpLoadBalancing,omitempty"`
	HorizontalPodAutoscaling bool `json:"horizontalPodAutoscaling,omitempty"`
	NetworkPolicyConfig      bool `json:"networkPolicyConfig,omitempty"`
}

type NodePoolConfig struct {
	Autoscaling       *NodePoolAutoscaling `json:"autoscaling,omitempty"`
	Config            *NodeConfig          `json:"config,omitempty"`
	InitialNodeCount  *int64               `json:"initialNodeCount,omitempty"`
	MaxPodsConstraint *int64               `json:"maxPodsConstraint,omitempty"`
	Name              *string              `json:"name,omitempty"`
	Version           *string              `json:"version,omitempty"`
}

type NodePoolAutoscaling struct {
	Enabled      bool  `json:"enabled,omitempty"`
	MaxNodeCount int64 `json:"maxNodeCount,omitempty"`
	MinNodeCount int64 `json:"minNodeCount,omitempty"`
}

type NodeConfig struct {
	DiskSizeGb    int64             `json:"diskSizeGb,omitempty"`
	DiskType      string            `json:"diskType,omitempty"`
	ImageType     string            `json:"imageType,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	LocalSsdCount int64             `json:"localSsdCount,omitempty"`
	MachineType   string            `json:"machineType,omitempty"`
	OauthScopes   []string          `json:"oauthScopes,omitempty"`
	Preemptible   bool              `json:"preemptible,omitempty"`
	Taints        []NodeTaintConfig `json:"taints,omitempty"`
}

type NodeTaintConfig struct {
	Effect string `json:"effect,omitempty"`
	Key    string `json:"key,omitempty"`
	Value  string `json:"value,omitempty"`
}

type MasterAuthorizedNetworksConfig struct {
	CidrBlocks []*CidrBlock `json:"cidrBlocks,omitempty"`
	Enabled    bool         `json:"enabled,omitempty"`
}

type CidrBlock struct {
	CidrBlock   string `json:"cidrBlock,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
}
