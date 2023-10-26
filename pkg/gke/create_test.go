package gke

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gkev1 "github.com/rancher/gke-operator/pkg/apis/gke.cattle.io/v1"
	"github.com/rancher/gke-operator/pkg/gke/services/mock_services"
	gkeapi "google.golang.org/api/container/v1"
)

var _ = Describe("CreateCluster", func() {
	var (
		mockController     *gomock.Controller
		clusterServiceMock *mock_services.MockGKEClusterService
		k8sVersion         = "1.25.12-gke.200"
		clusterIpv4Cidr    = "10.42.0.0/16"
		networkName        = "test-network"
		subnetworkName     = "test-subnetwork"
		emptyString        = ""
		boolTrue           = true
		nodePoolName       = "test-node-pool"
		initialNodeCount   = int64(3)
		maxPodsConstraint  = int64(110)
		config             = &gkev1.GKEClusterConfig{
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
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		clusterServiceMock = mock_services.NewMockGKEClusterService(mockController)
	})

	AfterEach(func() {
		mockController.Finish()
	})

	It("should successfully create cluster", func() {
		createClusterRequest := newClusterCreateRequest(config)
		clusterServiceMock.EXPECT().
			ClusterCreate(
				ctx,
				LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone)),
				createClusterRequest).
			Return(&gkeapi.Operation{}, nil)

		clusterServiceMock.EXPECT().
			ClusterList(
				ctx,
				LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone))).
			Return(&gkeapi.ListClustersResponse{}, nil)

		err := Create(ctx, clusterServiceMock, config)
		Expect(err).ToNot(HaveOccurred())

		clusterServiceMock.EXPECT().
			ClusterGet(
				ctx,
				ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone),
					config.Spec.ClusterName)).
			Return(
				&gkeapi.Cluster{
					Name: "test-cluster",
				}, nil)

		managedCluster, err := GetCluster(ctx, clusterServiceMock, &config.Spec)
		Expect(err).ToNot(HaveOccurred())
		Expect(managedCluster.Name).To(Equal(config.Spec.ClusterName))
	})

	It("should fail to create cluster", func() {
		clusterServiceMock.EXPECT().
			ClusterList(
				ctx,
				LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone))).
			Return(
				&gkeapi.ListClustersResponse{
					Clusters: []*gkeapi.Cluster{
						{
							Name: "test-cluster",
						},
					},
				}, nil)

		err := Create(ctx, clusterServiceMock, config)
		Expect(err).To(HaveOccurred())
	})

	It("should fail to create cluster with duplicated nodepool names", func() {
		config.Spec.NodePools = []gkev1.GKENodePoolConfig{
			{
				Name:              &nodePoolName,
				InitialNodeCount:  &initialNodeCount,
				Version:           &k8sVersion,
				MaxPodsConstraint: &maxPodsConstraint,
				Config:            &gkev1.GKENodeConfig{},
				Autoscaling: &gkev1.GKENodePoolAutoscaling{
					Enabled:      true,
					MinNodeCount: 3,
					MaxNodeCount: 5,
				},
				Management: &gkev1.GKENodePoolManagement{
					AutoRepair:  true,
					AutoUpgrade: true,
				},
			},
			{
				Name:              &nodePoolName,
				InitialNodeCount:  &initialNodeCount,
				Version:           &k8sVersion,
				MaxPodsConstraint: &maxPodsConstraint,
				Config:            &gkev1.GKENodeConfig{},
				Autoscaling: &gkev1.GKENodePoolAutoscaling{
					Enabled:      true,
					MinNodeCount: 3,
					MaxNodeCount: 5,
				},
				Management: &gkev1.GKENodePoolManagement{
					AutoRepair:  true,
					AutoUpgrade: true,
				},
			},
		}
		err := Create(ctx, clusterServiceMock, config)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("CreateNodePool", func() {
	var (
		mockController     *gomock.Controller
		clusterServiceMock *mock_services.MockGKEClusterService
		k8sVersion         = "1.25.12-gke.200"
		clusterIpv4Cidr    = "10.42.0.0/16"
		networkName        = "test-network"
		subnetworkName     = "test-subnetwork"
		emptyString        = ""
		boolTrue           = true

		nodePoolName      = "test-node-pool"
		initialNodeCount  = int64(3)
		maxPodsConstraint = int64(110)
		nodePoolConfig    = &gkev1.GKENodePoolConfig{
			Name:              &nodePoolName,
			InitialNodeCount:  &initialNodeCount,
			Version:           &k8sVersion,
			MaxPodsConstraint: &maxPodsConstraint,
			Config:            &gkev1.GKENodeConfig{},
			Autoscaling: &gkev1.GKENodePoolAutoscaling{
				Enabled:      true,
				MinNodeCount: 3,
				MaxNodeCount: 5,
			},
			Management: &gkev1.GKENodePoolManagement{
				AutoRepair:  true,
				AutoUpgrade: true,
			},
		}

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
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		clusterServiceMock = mock_services.NewMockGKEClusterService(mockController)
	})

	AfterEach(func() {
		mockController.Finish()
	})

	It("should successfully create cluster and node pool", func() {
		createClusterRequest := newClusterCreateRequest(config)
		clusterServiceMock.EXPECT().
			ClusterCreate(
				ctx,
				LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone)),
				createClusterRequest).
			Return(&gkeapi.Operation{}, nil)

		clusterServiceMock.EXPECT().
			ClusterList(
				ctx,
				LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone))).
			Return(&gkeapi.ListClustersResponse{}, nil)

		err := Create(ctx, clusterServiceMock, config)
		Expect(err).ToNot(HaveOccurred())

		createNodePoolRequest, err := newNodePoolCreateRequest(nodePoolConfig, config)
		Expect(err).ToNot(HaveOccurred())
		clusterServiceMock.EXPECT().
			NodePoolCreate(
				ctx,
				ClusterRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone), config.Spec.ClusterName),
				createNodePoolRequest).
			Return(&gkeapi.Operation{}, nil)

		status, err := CreateNodePool(ctx, clusterServiceMock, config, nodePoolConfig)
		Expect(err).ToNot(HaveOccurred())
		Expect(status).To(Equal(Changed))
	})
	It("shouldn't successfully create cluster and node pool", func() {
		testNodePoolConfig := &gkev1.GKENodePoolConfig{}
		status, err := CreateNodePool(ctx, clusterServiceMock, config, testNodePoolConfig)
		Expect(err).To(HaveOccurred())
		Expect(status).To(Equal(NotChanged))
	})
})
