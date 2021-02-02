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
	Region             string            `json:"region" norman:"noupdate"`
	Zone               string            `json:"zone" norman:"noupdate"`
	Imported           bool              `json:"imported" norman:"noupdate"`
	Description        string            `json:"description"`
	EnableAlphaFeature *bool             `json:"enableAlphaFeature"`
	ClusterAddons      ClusterAddons     `json:"clusterAddons"`
	ClusterIpv4Cidr    string            `json:"clusterIpv4Cidr"`
	ProjectID          string            `json:"projectID"`
	CredentialContent  string            `json:"credentialContent"`
	ClusterName        string            `json:"clusterName"`
	KubernetesVersion  *string           `json:"kubernetesVersion"`
	Tags               map[string]string `json:"tags"`
	SecretsEncryption  *bool             `json:"secretsEncryption" norman:"noupdate"`
	LoggingTypes       []string          `json:"loggingTypes"`
	Subnets            []string          `json:"subnets" norman:"noupdate"`
	SecurityGroups     []string          `json:"securityGroups" norman:"noupdate"`
	ServiceRole        *string           `json:"serviceRole" norman:"noupdate"`
	NodePools          []NodePoolConfig  `json:"nodePools"`
}

type GKEClusterConfigStatus struct {
	Phase          string   `json:"phase"`
	VirtualNetwork string   `json:"virtualNetwork"`
	Subnets        []string `json:"subnets"`
	SecurityGroups []string `json:"securityGroups"`
	// describes how the above network fields were provided. Valid values are provided and generated
	NetworkFieldsSource string `json:"networkFieldsSource"`
	FailureMessage      string `json:"failureMessage"`
}

type ClusterAddons struct {
	HTTPLoadBalancing        *bool `json:"httpLoadBalancing`
	HorizontalPodAutoscaling *bool `json:"horizontalPodAutoscaling"`
	KubernetesDashboard      *bool `json:"kubernetesDashboard"`
	NetworkPolicyConfig      *bool `json:"networkPolicyConfig"`
}

type NodePoolConfig struct {
	Autoscaling       *NodePoolAutoscaling `json:"autoscaling,omitempty"`
	Config            *NodeConfig          `json:"config,omitempty"`
	InitialNodeCount  *int64               `json:"initialNodeCount,omitempty"`
	MaxPodsConstraint *int32               `json:"maxPodsConstraint,omitempty"`
	Name              *string              `json:"name,omitempty"`
	Version           *string              `json:"version,omitempty"`
}

type NodePoolAutoscaling struct {
	Enabled      *bool  `json:"enabled,omitempty"`
	MaxNodeCount *int64 `json:"maxNodeCount,omitempty"`
	MinNodeCount *int64 `json:"minNodeCount,omitempty"`
}

type NodeConfig struct {
	DiskSizeGb    *int64             `json:"diskSizeGb,omitempty"`
	DiskType      *string            `json:"diskType,omitempty"`
	ImageType     *string            `json:"imageType,omitempty"`
	Labels        *map[string]string `json:"labels,omitempty"`
	LocalSsdCount *int64             `json:"localSsdCount,omitempty"`
	MachineType   *string            `json:"machineType,omitempty"`
	Preemptible   *bool              `json:"preemptible,omitempty"`
	Taints        []*NodeTaintConfig `json:"taints,omitempty"`
}

type NodeTaintConfig struct {
	Effect string `json:"effect,omitempty"`
	Key    string `json:"key,omitempty"`
	Value  string `json:"value,omitempty"`
}
