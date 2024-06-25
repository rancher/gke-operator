package gke

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gkev1 "github.com/rancher/gke-operator/pkg/apis/gke.cattle.io/v1"
	"github.com/rancher/gke-operator/pkg/gke/services/mock_services"
	gkeapi "google.golang.org/api/container/v1"
)

var _ = Describe("UpdateMasterKubernetesVersion", func() {
	var (
		mockController     *gomock.Controller
		clusterServiceMock *mock_services.MockGKEClusterService
		oldVersion         = "1.27.13-gke.3013000"
		k8sVersion         = "1.28.10-gke.1089000"
		clusterIpv4Cidr    = "10.42.0.0/16"
		networkName        = "test-network"
		subnetworkName     = "test-subnetwork"
		emptyString        = ""
		boolTrue           = true

		config = &gkev1.GKEClusterConfig{
			Spec: gkev1.GKEClusterConfigSpec{
				Region:                "test-region",
				ProjectID:             "test-project",
				ClusterName:           "test-cluster",
				Locations:             []string{""},
				Labels:                map[string]string{"test": "test"},
				ClusterIpv4CidrBlock:  &clusterIpv4Cidr,
				KubernetesVersion:     &k8sVersion,
				LoggingService:        &emptyString,
				MonitoringService:     &emptyString,
				EnableKubernetesAlpha: &boolTrue,
				Network:               &networkName,
				Subnetwork:            &subnetworkName,
				NetworkPolicyEnabled:  &boolTrue,
				MaintenanceWindow:     &emptyString,
				IPAllocationPolicy: &gkev1.GKEIPAllocationPolicy{
					UseIPAliases: true,
				},
				ClusterAddons: &gkev1.GKEClusterAddons{
					HTTPLoadBalancing:        true,
					NetworkPolicyConfig:      false,
					HorizontalPodAutoscaling: true,
				},
				PrivateClusterConfig: &gkev1.GKEPrivateClusterConfig{
					EnablePrivateEndpoint: false,
					EnablePrivateNodes:    false,
				},
				MasterAuthorizedNetworksConfig: &gkev1.GKEMasterAuthorizedNetworksConfig{
					Enabled: false,
				},
			},
		}
		upstreamSpec = &gkev1.GKEClusterConfigSpec{
			ClusterName:       "test-cluster",
			KubernetesVersion: &oldVersion,
		}
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		clusterServiceMock = mock_services.NewMockGKEClusterService(mockController)
	})

	AfterEach(func() {
		mockController.Finish()
	})

	It("should update cluster version", func() {
		clusterServiceMock.EXPECT().
			ClusterUpdate(
				ctx,
				ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName),
				&gkeapi.UpdateClusterRequest{
					Update: &gkeapi.ClusterUpdate{
						DesiredMasterVersion: k8sVersion,
					},
				}).
			Return(&gkeapi.Operation{}, nil)

		status, err := UpdateMasterKubernetesVersion(ctx, clusterServiceMock, config, upstreamSpec)
		Expect(err).ToNot(HaveOccurred())
		Expect(status).To(Equal(Changed))
	})

	It("should not update cluster version", func() {
		upstreamSpec.KubernetesVersion = &k8sVersion
		status, err := UpdateMasterKubernetesVersion(ctx, clusterServiceMock, config, upstreamSpec)
		Expect(err).ToNot(HaveOccurred())
		Expect(status).To(Equal(NotChanged))
	})

})

var _ = Describe("UpdateClusterAddons", func() {
	var (
		mockController     *gomock.Controller
		clusterServiceMock *mock_services.MockGKEClusterService
		k8sVersion         = "1.28.10-gke.1089000"
		clusterIpv4Cidr    = "10.42.0.0/16"
		networkName        = "test-network"
		subnetworkName     = "test-subnetwork"
		emptyString        = ""
		boolTrue           = true

		config = &gkev1.GKEClusterConfig{
			Spec: gkev1.GKEClusterConfigSpec{
				Region:                "test-region",
				ProjectID:             "test-project",
				ClusterName:           "test-cluster",
				Locations:             []string{""},
				Labels:                map[string]string{"test": "test"},
				ClusterIpv4CidrBlock:  &clusterIpv4Cidr,
				KubernetesVersion:     &k8sVersion,
				LoggingService:        &emptyString,
				MonitoringService:     &emptyString,
				EnableKubernetesAlpha: &boolTrue,
				Network:               &networkName,
				Subnetwork:            &subnetworkName,
				NetworkPolicyEnabled:  &boolTrue,
				MaintenanceWindow:     &emptyString,
				IPAllocationPolicy: &gkev1.GKEIPAllocationPolicy{
					UseIPAliases: true,
				},
				ClusterAddons: &gkev1.GKEClusterAddons{
					HTTPLoadBalancing:        true,
					NetworkPolicyConfig:      false,
					HorizontalPodAutoscaling: true,
				},
				PrivateClusterConfig: &gkev1.GKEPrivateClusterConfig{
					EnablePrivateEndpoint: false,
					EnablePrivateNodes:    false,
				},
				MasterAuthorizedNetworksConfig: &gkev1.GKEMasterAuthorizedNetworksConfig{
					Enabled: false,
				},
			},
		}
		upstreamSpec = &gkev1.GKEClusterConfigSpec{
			ClusterName: "test-cluster",
			ClusterAddons: &gkev1.GKEClusterAddons{
				HTTPLoadBalancing:        false,
				NetworkPolicyConfig:      true,
				HorizontalPodAutoscaling: false,
			},
		}
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		clusterServiceMock = mock_services.NewMockGKEClusterService(mockController)
	})

	AfterEach(func() {
		mockController.Finish()
	})

	It("should change addons", func() {
		clusterServiceMock.EXPECT().
			ClusterUpdate(
				ctx,
				ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName),
				&gkeapi.UpdateClusterRequest{
					Update: &gkeapi.ClusterUpdate{
						DesiredAddonsConfig: &gkeapi.AddonsConfig{
							HttpLoadBalancing: &gkeapi.HttpLoadBalancing{
								Disabled: false,
							},
							NetworkPolicyConfig: &gkeapi.NetworkPolicyConfig{
								Disabled: true,
							},
							HorizontalPodAutoscaling: &gkeapi.HorizontalPodAutoscaling{
								Disabled: false,
							},
						},
					},
				}).
			Return(&gkeapi.Operation{}, nil)

		status, err := UpdateClusterAddons(ctx, clusterServiceMock, config, upstreamSpec)
		Expect(err).ToNot(HaveOccurred())
		Expect(status).To(Equal(Changed))
	})

	It("should not change addons", func() {
		upstreamSpec.ClusterAddons.HTTPLoadBalancing = true
		upstreamSpec.ClusterAddons.NetworkPolicyConfig = false
		upstreamSpec.ClusterAddons.HorizontalPodAutoscaling = true
		status, err := UpdateClusterAddons(ctx, clusterServiceMock, config, upstreamSpec)
		Expect(err).ToNot(HaveOccurred())
		Expect(status).To(Equal(NotChanged))
	})

})

var _ = Describe("UpdateMasterAuthorizedNetworks", func() {
	var (
		mockController     *gomock.Controller
		clusterServiceMock *mock_services.MockGKEClusterService
		k8sVersion         = "1.28.10-gke.1089000"
		clusterIpv4Cidr    = "10.42.0.0/16"
		networkName        = "test-network"
		subnetworkName     = "test-subnetwork"
		emptyString        = ""
		boolTrue           = true

		config = &gkev1.GKEClusterConfig{
			Spec: gkev1.GKEClusterConfigSpec{
				Region:                "test-region",
				ProjectID:             "test-project",
				ClusterName:           "test-cluster",
				Locations:             []string{""},
				Labels:                map[string]string{"test": "test"},
				ClusterIpv4CidrBlock:  &clusterIpv4Cidr,
				KubernetesVersion:     &k8sVersion,
				LoggingService:        &emptyString,
				MonitoringService:     &emptyString,
				EnableKubernetesAlpha: &boolTrue,
				Network:               &networkName,
				Subnetwork:            &subnetworkName,
				NetworkPolicyEnabled:  &boolTrue,
				MaintenanceWindow:     &emptyString,
				IPAllocationPolicy: &gkev1.GKEIPAllocationPolicy{
					UseIPAliases: true,
				},
				ClusterAddons: &gkev1.GKEClusterAddons{
					HTTPLoadBalancing:        true,
					NetworkPolicyConfig:      false,
					HorizontalPodAutoscaling: true,
				},
				PrivateClusterConfig: &gkev1.GKEPrivateClusterConfig{
					EnablePrivateEndpoint: false,
					EnablePrivateNodes:    false,
				},
				MasterAuthorizedNetworksConfig: &gkev1.GKEMasterAuthorizedNetworksConfig{
					Enabled: true,
					CidrBlocks: []*gkev1.GKECidrBlock{
						{
							CidrBlock:   "10.10.10.0/24",
							DisplayName: "test-auth-network",
						},
					},
				},
			},
		}
		upstreamSpec = &gkev1.GKEClusterConfigSpec{
			ClusterName: "test-cluster",
			MasterAuthorizedNetworksConfig: &gkev1.GKEMasterAuthorizedNetworksConfig{
				Enabled: false,
				CidrBlocks: []*gkev1.GKECidrBlock{
					{
						CidrBlock:   "10.10.20.0/24",
						DisplayName: "test-auth-network",
					},
				},
			},
		}
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		clusterServiceMock = mock_services.NewMockGKEClusterService(mockController)
	})

	AfterEach(func() {
		mockController.Finish()
	})

	It("should change authorized networks", func() {
		clusterServiceMock.EXPECT().
			ClusterUpdate(
				ctx,
				ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName),
				&gkeapi.UpdateClusterRequest{
					Update: &gkeapi.ClusterUpdate{
						DesiredMasterAuthorizedNetworksConfig: &gkeapi.MasterAuthorizedNetworksConfig{
							Enabled: true,
							CidrBlocks: []*gkeapi.CidrBlock{
								{
									CidrBlock:   "10.10.10.0/24",
									DisplayName: "test-auth-network",
								},
							},
						},
					},
				}).
			Return(&gkeapi.Operation{}, nil)

		status, err := UpdateMasterAuthorizedNetworks(ctx, clusterServiceMock, config, upstreamSpec)
		Expect(err).ToNot(HaveOccurred())
		Expect(status).To(Equal(Changed))
	})

	It("should not change authorized networks", func() {
		upstreamSpec.MasterAuthorizedNetworksConfig.Enabled = true
		upstreamSpec.MasterAuthorizedNetworksConfig.CidrBlocks = []*gkev1.GKECidrBlock{
			{
				CidrBlock:   "10.10.10.0/24",
				DisplayName: "test-auth-network",
			},
		}
		status, err := UpdateMasterAuthorizedNetworks(ctx, clusterServiceMock, config, upstreamSpec)
		Expect(err).ToNot(HaveOccurred())
		Expect(status).To(Equal(NotChanged))
	})

})

var _ = Describe("UpdateNodePoolAutoscaling", func() {
	var (
		mockController     *gomock.Controller
		clusterServiceMock *mock_services.MockGKEClusterService
		k8sVersion         = "1.28.10-gke.1089000"
		clusterIpv4Cidr    = "10.42.0.0/16"
		networkName        = "test-network"
		subnetworkName     = "test-subnetwork"
		emptyString        = ""
		boolTrue           = true
		nodePoolName       = "test-node-pool"
		initialNodeCount   = int64(1)
		maxPodsConstraint  = int64(100)

		config = &gkev1.GKEClusterConfig{
			Spec: gkev1.GKEClusterConfigSpec{
				Region:                "test-region",
				ProjectID:             "test-project",
				ClusterName:           "test-cluster",
				Locations:             []string{""},
				Labels:                map[string]string{"test": "test"},
				ClusterIpv4CidrBlock:  &clusterIpv4Cidr,
				KubernetesVersion:     &k8sVersion,
				LoggingService:        &emptyString,
				MonitoringService:     &emptyString,
				EnableKubernetesAlpha: &boolTrue,
				Network:               &networkName,
				Subnetwork:            &subnetworkName,
				NetworkPolicyEnabled:  &boolTrue,
				MaintenanceWindow:     &emptyString,
				IPAllocationPolicy: &gkev1.GKEIPAllocationPolicy{
					UseIPAliases: true,
				},
				ClusterAddons: &gkev1.GKEClusterAddons{
					HTTPLoadBalancing:        true,
					NetworkPolicyConfig:      false,
					HorizontalPodAutoscaling: true,
				},
				PrivateClusterConfig: &gkev1.GKEPrivateClusterConfig{
					EnablePrivateEndpoint: false,
					EnablePrivateNodes:    false,
				},
				MasterAuthorizedNetworksConfig: &gkev1.GKEMasterAuthorizedNetworksConfig{
					Enabled: false,
				},
			},
		}
		nodePool = &gkev1.GKENodePoolConfig{
			Name: &nodePoolName,
			Config: &gkev1.GKENodeConfig{
				DiskSizeGb:  100,
				DiskType:    "pg-balanced",
				ImageType:   "COS_CONTAINERD",
				MachineType: "n2-standard-2",
			},
			InitialNodeCount: &initialNodeCount,
			Management: &gkev1.GKENodePoolManagement{
				AutoRepair:  true,
				AutoUpgrade: true,
			},
			MaxPodsConstraint: &maxPodsConstraint,
		}

		upstreamNodePool = &gkev1.GKENodePoolConfig{
			Name: &nodePoolName,
			Config: &gkev1.GKENodeConfig{
				DiskSizeGb:  100,
				DiskType:    "pg-balanced",
				ImageType:   "COS_CONTAINERD",
				MachineType: "n2-standard-2",
			},
			InitialNodeCount: &initialNodeCount,
			Management: &gkev1.GKENodePoolManagement{
				AutoRepair:  true,
				AutoUpgrade: true,
			},
			MaxPodsConstraint: &maxPodsConstraint,
		}
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		clusterServiceMock = mock_services.NewMockGKEClusterService(mockController)
	})

	AfterEach(func() {
		mockController.Finish()
	})

	It("NodePool autoscaling shouldn't change when GKE Config is nil", func() {
		status, err := UpdateNodePoolAutoscaling(ctx, clusterServiceMock, nodePool, config, upstreamNodePool)
		Expect(err).ToNot(HaveOccurred())
		Expect(status).To(Equal(NotChanged))
	})

	It("NodePool autoscaling shouldn't change when upstreamSpec is set and GKE Config is nil", func() {
		upstreamNodePool.Autoscaling = &gkev1.GKENodePoolAutoscaling{
			Enabled:      true,
			MinNodeCount: 1,
			MaxNodeCount: 10,
		}

		status, err := UpdateNodePoolAutoscaling(ctx, clusterServiceMock, nodePool, config, upstreamNodePool)
		Expect(err).ToNot(HaveOccurred())
		Expect(status).To(Equal(NotChanged))
	})

	It("NodePool autoscaling should change when upstreamSpec is nil and GKE Config is set", func() {
		nodePool.Autoscaling = &gkev1.GKENodePoolAutoscaling{
			Enabled:      true,
			MinNodeCount: 1,
			MaxNodeCount: 10,
		}

		clusterServiceMock.EXPECT().
			SetAutoscaling(
				ctx,
				NodePoolRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName, *nodePool.Name),
				&gkeapi.SetNodePoolAutoscalingRequest{
					Autoscaling: &gkeapi.NodePoolAutoscaling{
						Enabled:      true,
						MinNodeCount: 1,
						MaxNodeCount: 10,
					},
				}).
			Return(&gkeapi.Operation{}, nil)

		status, err := UpdateNodePoolAutoscaling(ctx, clusterServiceMock, nodePool, config, upstreamNodePool)
		Expect(err).ToNot(HaveOccurred())
		Expect(status).To(Equal(Changed))
	})

	It("NodePool autoscaling should change when upstreamSpec is different than GKE Config", func() {
		nodePool.Autoscaling = &gkev1.GKENodePoolAutoscaling{
			Enabled:      true,
			MinNodeCount: 1,
			MaxNodeCount: 10,
		}
		upstreamNodePool.Autoscaling = &gkev1.GKENodePoolAutoscaling{
			Enabled:      true,
			MinNodeCount: 3,
			MaxNodeCount: 5,
		}

		clusterServiceMock.EXPECT().
			SetAutoscaling(
				ctx,
				NodePoolRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName, *nodePool.Name),
				&gkeapi.SetNodePoolAutoscalingRequest{
					Autoscaling: &gkeapi.NodePoolAutoscaling{
						Enabled:      true,
						MinNodeCount: 1,
						MaxNodeCount: 10,
					},
				}).
			Return(&gkeapi.Operation{}, nil)

		status, err := UpdateNodePoolAutoscaling(ctx, clusterServiceMock, nodePool, config, upstreamNodePool)
		Expect(err).ToNot(HaveOccurred())
		Expect(status).To(Equal(Changed))
	})

})
