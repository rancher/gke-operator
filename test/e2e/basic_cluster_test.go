/*
Copyright © 2024 SUSE LLC

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

package e2e

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gkev1 "github.com/rancher/gke-operator/pkg/apis/gke.cattle.io/v1"
	managementv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("BasicCluster", func() {
	var (
		gkeConfig *gkev1.GKEClusterConfig
		cluster   *managementv3.Cluster
	)

	BeforeEach(func() {
		var ok bool
		gkeConfig, ok = clusterTemplates[basicClusterTemplateName]
		Expect(ok).To(BeTrue())
		Expect(gkeConfig).NotTo(BeNil())

		cluster = &managementv3.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: gkeConfig.Name,
			},
			Spec: managementv3.ClusterSpec{
				GKEConfig: &gkeConfig.Spec,
			},
		}

	})

	It("Succesfully creates a cluster", func() {
		By("Creating a cluster")
		Expect(cl.Create(ctx, cluster)).Should(Succeed())

		By("Waiting for cluster to be ready")
		Eventually(func() error {
			currentCluster := &gkev1.GKEClusterConfig{}

			if err := cl.Get(ctx, runtimeclient.ObjectKey{
				Name:      cluster.Name,
				Namespace: gkeClusterConfigNamespace,
			}, currentCluster); err != nil {
				return err
			}

			if currentCluster.Status.Phase == "active" {
				return nil
			}

			return fmt.Errorf("cluster is not ready yet. Current phase: %s", currentCluster.Status.Phase)
		}, waitLong, pollInterval).ShouldNot(HaveOccurred())
	})

	It("Successfully adds and removes a node pool", func() {
		initialNodePools := gkeConfig.DeepCopy().Spec.NodePools // save to restore later and test deletion

		Expect(cl.Get(ctx, runtimeclient.ObjectKey{Name: cluster.Name}, cluster)).Should(Succeed())
		patch := runtimeclient.MergeFrom(cluster.DeepCopy())

		nodePoolName := "gke-e2e-additional-node-pool"
		initialNodeCount := int64(1)
		maxPodsConstraint := int64(110)
		nodePool := gkev1.GKENodePoolConfig{
			Name:              &nodePoolName,
			InitialNodeCount:  &initialNodeCount,
			Version:           gkeConfig.Spec.KubernetesVersion,
			MaxPodsConstraint: &maxPodsConstraint,
			Config:            &gkev1.GKENodeConfig{},
			Autoscaling: &gkev1.GKENodePoolAutoscaling{
				Enabled:      true,
				MinNodeCount: 1,
				MaxNodeCount: 2,
			},
			Management: &gkev1.GKENodePoolManagement{
				AutoRepair:  true,
				AutoUpgrade: true,
			},
		}

		cluster.Spec.GKEConfig.NodePools = append(cluster.Spec.GKEConfig.NodePools, nodePool)

		Expect(cl.Patch(ctx, cluster, patch)).Should(Succeed())

		By("Waiting for cluster to start adding node pool")
		Eventually(func() error {
			currentCluster := &gkev1.GKEClusterConfig{}

			if err := cl.Get(ctx, runtimeclient.ObjectKey{
				Name:      cluster.Name,
				Namespace: gkeClusterConfigNamespace,
			}, currentCluster); err != nil {
				return err
			}

			if currentCluster.Status.Phase == "updating" && len(currentCluster.Spec.NodePools) == 2 {
				return nil
			}

			return fmt.Errorf("cluster didn't get new node pool. Current phase: %s, node pool count %d", currentCluster.Status.Phase, len(currentCluster.Spec.NodePools))
		}, waitLong, pollInterval).ShouldNot(HaveOccurred())

		By("Waiting for cluster to finish adding node pool")
		Eventually(func() error {
			currentCluster := &gkev1.GKEClusterConfig{}

			if err := cl.Get(ctx, runtimeclient.ObjectKey{
				Name:      cluster.Name,
				Namespace: gkeClusterConfigNamespace,
			}, currentCluster); err != nil {
				return err
			}

			if currentCluster.Status.Phase == "active" && len(currentCluster.Spec.NodePools) == 2 {
				return nil
			}

			return fmt.Errorf("cluster didn't finish adding node pool. Current phase: %s, node pool count %d", currentCluster.Status.Phase, len(currentCluster.Spec.NodePools))
		}, waitLong, pollInterval).ShouldNot(HaveOccurred())

		By("Restoring initial node pools")

		Expect(cl.Get(ctx, runtimeclient.ObjectKey{Name: cluster.Name}, cluster)).Should(Succeed())
		patch = runtimeclient.MergeFrom(cluster.DeepCopy())

		cluster.Spec.GKEConfig.NodePools = initialNodePools

		Expect(cl.Patch(ctx, cluster, patch)).Should(Succeed())

		By("Waiting for cluster to start removing node pool")
		Eventually(func() error {
			currentCluster := &gkev1.GKEClusterConfig{}

			if err := cl.Get(ctx, runtimeclient.ObjectKey{
				Name:      cluster.Name,
				Namespace: gkeClusterConfigNamespace,
			}, currentCluster); err != nil {
				return err
			}

			if currentCluster.Status.Phase == "updating" && len(currentCluster.Spec.NodePools) == 1 {
				return nil
			}

			return fmt.Errorf("cluster didn't start removing node pool. Current phase: %s, node pool count %d", currentCluster.Status.Phase, len(currentCluster.Spec.NodePools))
		}, waitLong, pollInterval).ShouldNot(HaveOccurred())

		By("Waiting for cluster to finish removing node pool")
		Eventually(func() error {
			currentCluster := &gkev1.GKEClusterConfig{}

			if err := cl.Get(ctx, runtimeclient.ObjectKey{
				Name:      cluster.Name,
				Namespace: gkeClusterConfigNamespace,
			}, currentCluster); err != nil {
				return err
			}

			if currentCluster.Status.Phase == "active" && len(currentCluster.Spec.NodePools) == 1 {
				return nil
			}

			return fmt.Errorf("cluster didn't finish removing node pool. Current phase: %s, node pool count %d", currentCluster.Status.Phase, len(currentCluster.Spec.NodePools))
		}, waitLong, pollInterval).ShouldNot(HaveOccurred())

		By("Done waiting for cluster to finish removing node pool")
	})

})
