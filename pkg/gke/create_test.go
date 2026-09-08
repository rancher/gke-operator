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
		serverConfig       = &gkeapi.ServerConfig{}
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
		serverConfig.Channels = []*gkeapi.ReleaseChannelConfig{
			{
				Channel:       "REGULAR",
				ValidVersions: []string{k8sVersion},
			},
		}
		clusterServiceMock.EXPECT().
			ServerConfigGet(
				ctx,
				LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone))).
			Return(serverConfig, nil).
			AnyTimes()
	})

	AfterEach(func() {
		mockController.Finish()
	})

	It("should successfully create cluster", func() {
		createClusterRequest := NewClusterCreateRequest(config)
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

	It("should successfully create cluster with customer managment encryption key", func() {
		config.Spec.CustomerManagedEncryptionKey = &gkev1.CMEKConfig{
			KeyName:  "test-key",
			RingName: "test-keyring",
		}
		createClusterRequest := NewClusterCreateRequest(config)
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

	It("should successfully create autopilot cluster", func() {
		config.Spec.ClusterName = "test-autopilot-cluster"
		config.Spec.AutopilotConfig = &gkev1.GKEAutopilotConfig{
			Enabled: true,
		}

		createClusterRequest := NewClusterCreateRequest(config)
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
					Name: "test-autopilot-cluster",
				}, nil)

		managedCluster, err := GetCluster(ctx, clusterServiceMock, &config.Spec)
		Expect(err).ToNot(HaveOccurred())
		Expect(managedCluster.Name).To(Equal(config.Spec.ClusterName))
	})

	It("should fail create cluster with customer managment encryption key", func() {
		config.Spec.CustomerManagedEncryptionKey = &gkev1.CMEKConfig{
			KeyName: "test-key",
		}
		err := Create(ctx, clusterServiceMock, config)
		Expect(err).To(HaveOccurred())
	})

	It("should fail to create autopilot cluster with nodepools", func() {
		config.Spec.ClusterName = "test-autopilot-cluster"
		config.Spec.AutopilotConfig = &gkev1.GKEAutopilotConfig{
			Enabled: true,
		}

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
		}

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

	It("should default release channel to REGULAR when omitted", func() {
		request := NewClusterCreateRequest(config)
		Expect(request.Cluster.ReleaseChannel).ToNot(BeNil())
		Expect(request.Cluster.ReleaseChannel.Channel).To(Equal("REGULAR"))
	})

	It("should map release channels to expected GKE values", func() {
		tests := []struct {
			specValue gkev1.GKEReleaseChannel
			expected  string
		}{
			{specValue: gkev1.GKEReleaseChannelRapid, expected: "RAPID"},
			{specValue: gkev1.GKEReleaseChannelRegular, expected: "REGULAR"},
			{specValue: gkev1.GKEReleaseChannelStable, expected: "STABLE"},
			{specValue: gkev1.GKEReleaseChannelExtended, expected: "EXTENDED"},
		}

		for _, test := range tests {
			c := *config.DeepCopy()
			c.Spec.ReleaseChannel = &test.specValue
			request := NewClusterCreateRequest(&c)
			Expect(request.Cluster.ReleaseChannel).ToNot(BeNil())
			Expect(request.Cluster.ReleaseChannel.Channel).To(Equal(test.expected))
		}
	})

	It("should resolve release channel from version when empty release channel is provided", func() {
		config.Spec.AutopilotConfig = nil
		config.Spec.NodePools = nil
		config.Spec.CustomerManagedEncryptionKey = nil
		config.Spec.ClusterName = "test-cluster"
		emptyChannel := gkev1.GKEReleaseChannel("")
		config.Spec.ReleaseChannel = &emptyChannel
		expectedConfig := config.DeepCopy()
		regular := gkev1.GKEReleaseChannelRegular
		expectedConfig.Spec.ReleaseChannel = &regular
		createClusterRequest := NewClusterCreateRequest(expectedConfig)

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
		Expect(config.Spec.ReleaseChannel).ToNot(BeNil())
		Expect(*config.Spec.ReleaseChannel).To(Equal(gkev1.GKEReleaseChannelRegular))
	})

	It("should reject an invalid release channel instead of silently falling back to REGULAR", func() {
		config.Spec.AutopilotConfig = nil
		config.Spec.NodePools = nil
		config.Spec.CustomerManagedEncryptionKey = nil
		config.Spec.ClusterName = "test-cluster"
		invalidChannel := gkev1.GKEReleaseChannel("foo")
		config.Spec.ReleaseChannel = &invalidChannel

		clusterServiceMock.EXPECT().
			ClusterList(
				ctx,
				LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone))).
			Return(&gkeapi.ListClustersResponse{}, nil)

		err := Create(ctx, clusterServiceMock, config)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid release channel"))

		config.Spec.ReleaseChannel = nil
	})

	It("should use the explicitly selected release channel when the version is supported, even if a higher-priority channel also supports it", func() {
		config.Spec.AutopilotConfig = nil
		config.Spec.NodePools = nil
		config.Spec.CustomerManagedEncryptionKey = nil
		config.Spec.ClusterName = "test-cluster"
		extended := gkev1.GKEReleaseChannelExtended
		config.Spec.ReleaseChannel = &extended
		// STABLE also supports this version, but the user explicitly asked for EXTENDED, so
		// EXTENDED must be what's actually provisioned, not the "safer" STABLE channel.
		serverConfig.Channels = []*gkeapi.ReleaseChannelConfig{
			{
				Channel:       "STABLE",
				ValidVersions: []string{k8sVersion},
			},
			{
				Channel:       "EXTENDED",
				ValidVersions: []string{k8sVersion},
			},
		}

		createClusterRequest := NewClusterCreateRequest(config)
		Expect(createClusterRequest.Cluster.ReleaseChannel.Channel).To(Equal("EXTENDED"))

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
		Expect(config.Spec.ReleaseChannel).ToNot(BeNil())
		Expect(*config.Spec.ReleaseChannel).To(Equal(gkev1.GKEReleaseChannelExtended))
	})

	It("should reject an explicitly selected release channel when the version isn't supported by it", func() {
		config.Spec.AutopilotConfig = nil
		config.Spec.NodePools = nil
		config.Spec.CustomerManagedEncryptionKey = nil
		config.Spec.ClusterName = "test-cluster"
		extended := gkev1.GKEReleaseChannelExtended
		config.Spec.ReleaseChannel = &extended
		// The version is only available in STABLE, not the requested EXTENDED channel: this
		// must be rejected rather than silently falling back to STABLE.
		serverConfig.Channels = []*gkeapi.ReleaseChannelConfig{
			{
				Channel:       "STABLE",
				ValidVersions: []string{k8sVersion},
			},
		}

		clusterServiceMock.EXPECT().
			ClusterList(
				ctx,
				LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone))).
			Return(&gkeapi.ListClustersResponse{}, nil)

		err := Create(ctx, clusterServiceMock, config)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not supported by releaseChannel"))

		config.Spec.ReleaseChannel = nil
	})

	It("should derive release channel from version using priority order", func() {
		config.Spec.AutopilotConfig = nil
		config.Spec.NodePools = nil
		config.Spec.CustomerManagedEncryptionKey = nil
		config.Spec.ClusterName = "test-cluster"
		config.Spec.ReleaseChannel = nil
		serverConfig.Channels = []*gkeapi.ReleaseChannelConfig{
			{
				Channel:       "STABLE",
				ValidVersions: []string{k8sVersion},
			},
			{
				Channel:       "REGULAR",
				ValidVersions: []string{k8sVersion},
			},
			{
				Channel:       "RAPID",
				ValidVersions: []string{k8sVersion},
			},
		}

		expectedConfig := config.DeepCopy()
		stable := gkev1.GKEReleaseChannelStable
		expectedConfig.Spec.ReleaseChannel = &stable
		createClusterRequest := NewClusterCreateRequest(expectedConfig)

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
		Expect(config.Spec.ReleaseChannel).ToNot(BeNil())
		Expect(*config.Spec.ReleaseChannel).To(Equal(gkev1.GKEReleaseChannelStable))
	})

	It("should fail when version is not found in any release channel", func() {
		config.Spec.AutopilotConfig = nil
		config.Spec.NodePools = nil
		config.Spec.CustomerManagedEncryptionKey = nil
		config.Spec.ClusterName = "test-cluster"
		config.Spec.ReleaseChannel = nil
		serverConfig.Channels = []*gkeapi.ReleaseChannelConfig{
			{
				Channel:       "STABLE",
				ValidVersions: []string{"1.99.99-gke.999"},
			},
		}

		clusterServiceMock.EXPECT().
			ClusterList(
				ctx,
				LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone))).
			Return(&gkeapi.ListClustersResponse{}, nil)

		err := Create(ctx, clusterServiceMock, config)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("cannot resolve releaseChannel for kubernetesVersion"))
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
		serverConfig       = &gkeapi.ServerConfig{}

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
		serverConfig.Channels = []*gkeapi.ReleaseChannelConfig{
			{
				Channel:       "REGULAR",
				ValidVersions: []string{k8sVersion},
			},
		}
		clusterServiceMock.EXPECT().
			ServerConfigGet(
				ctx,
				LocationRRN(config.Spec.ProjectID, Location(config.Spec.Region, config.Spec.Zone))).
			Return(serverConfig, nil).
			AnyTimes()
	})

	AfterEach(func() {
		mockController.Finish()
	})

	It("should successfully create cluster and node pool", func() {
		createClusterRequest := NewClusterCreateRequest(config)
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
