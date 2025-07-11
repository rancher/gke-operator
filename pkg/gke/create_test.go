package gke

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gkeapi "google.golang.org/api/container/v1"

	gkev1 "github.com/rancher/gke-operator/pkg/apis/gke.cattle.io/v1"
	"github.com/rancher/gke-operator/pkg/gke/services/mock_services"
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
		
		// Reset config to clean state for each test
		config.Spec.NodePools = nil
		config.Spec.CustomerManagedEncryptionKey = nil
		config.Spec.AutopilotConfig = nil
		config.Spec.ClusterName = "test-cluster"
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
	It("should successfully create cluster with customer managed encryption key from different project", func() {
		// Add a node pool to the config so we can test CMEK
		nodePoolVersion := "1.25.12-gke.200"
		config.Spec.NodePools = []gkev1.GKENodePoolConfig{
			{
				Name:              &nodePoolName,
				InitialNodeCount:  &initialNodeCount,
				MaxPodsConstraint: &maxPodsConstraint,
				Version:           &nodePoolVersion,
				Config:            &gkev1.GKENodeConfig{},
				Autoscaling: &gkev1.GKENodePoolAutoscaling{
					Enabled:      false,
					MinNodeCount: 1,
					MaxNodeCount: 3,
				},
				Management: &gkev1.GKENodePoolManagement{
					AutoRepair:  true,
					AutoUpgrade: true,
				},
			},
		}
		
		config.Spec.CustomerManagedEncryptionKey = &gkev1.CMEKConfig{
			KeyName:   "test-key",
			RingName:  "test-keyring",
			ProjectID: "security-project-id", // Different from cluster project
		}
		createClusterRequest := NewClusterCreateRequest(config)
		
		// Verify the BootDiskKmsKey uses the security project ID, not the cluster project ID
		nodePool := createClusterRequest.Cluster.NodePools[0]
		expectedKeyPath := "projects/security-project-id/locations/test-region/keyRings/test-keyring/cryptoKeys/test-key"
		Expect(nodePool.Config.BootDiskKmsKey).To(Equal(expectedKeyPath))

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

	It("should successfully create cluster with customer managed encryption key from different project and region", func() {
		// Add a node pool to the config so we can test CMEK
		nodePoolVersion := "1.25.12-gke.200"
		config.Spec.NodePools = []gkev1.GKENodePoolConfig{
			{
				Name:              &nodePoolName,
				InitialNodeCount:  &initialNodeCount,
				MaxPodsConstraint: &maxPodsConstraint,
				Version:           &nodePoolVersion,
				Config:            &gkev1.GKENodeConfig{},
				Autoscaling: &gkev1.GKENodePoolAutoscaling{
					Enabled:      false,
					MinNodeCount: 1,
					MaxNodeCount: 3,
				},
				Management: &gkev1.GKENodePoolManagement{
					AutoRepair:  true,
					AutoUpgrade: true,
				},
			},
		}
		
		config.Spec.CustomerManagedEncryptionKey = &gkev1.CMEKConfig{
			KeyName:   "test-vmenc-key-us-central1",
			RingName:  "test-keyring-us-central1",
			ProjectID: "security-project-id", // Different from cluster project
			Region:    "us-central1",         // Explicit region for key (cluster might be in us-west1)
		}
		createClusterRequest := NewClusterCreateRequest(config)
		
		// Verify the BootDiskKmsKey uses the specified project and region
		nodePool := createClusterRequest.Cluster.NodePools[0]
		expectedKeyPath := "projects/security-project-id/locations/us-central1/keyRings/test-keyring-us-central1/cryptoKeys/test-vmenc-key-us-central1"
		Expect(nodePool.Config.BootDiskKmsKey).To(Equal(expectedKeyPath))
		
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

	It("should successfully create cluster with customer managed encryption key from different project, region, and zone", func() {
		// Add a node pool to the config so we can test CMEK
		nodePoolVersion := "1.25.12-gke.200"
		config.Spec.NodePools = []gkev1.GKENodePoolConfig{
			{
				Name:              &nodePoolName,
				InitialNodeCount:  &initialNodeCount,
				MaxPodsConstraint: &maxPodsConstraint,
				Version:           &nodePoolVersion,
				Config:            &gkev1.GKENodeConfig{},
				Autoscaling: &gkev1.GKENodePoolAutoscaling{
					Enabled:      false,
					MinNodeCount: 1,
					MaxNodeCount: 3,
				},
				Management: &gkev1.GKENodePoolManagement{
					AutoRepair:  true,
					AutoUpgrade: true,
				},
			},
		}
		
		config.Spec.CustomerManagedEncryptionKey = &gkev1.CMEKConfig{
			KeyName:   "zonal-test-key",
			RingName:  "zonal-test-keyring",
			ProjectID: "test-kms-project",   // Different project
			Region:    "us-east1",           // Different region
			Zone:      "us-east1-b",         // Specific zone for zonal key
		}
		createClusterRequest := NewClusterCreateRequest(config)
		
		// Verify the BootDiskKmsKey uses the specified project, region, and zone
		nodePool := createClusterRequest.Cluster.NodePools[0]
		expectedKeyPath := "projects/test-kms-project/locations/us-east1-b/keyRings/zonal-test-keyring/cryptoKeys/zonal-test-key"
		Expect(nodePool.Config.BootDiskKmsKey).To(Equal(expectedKeyPath))
		
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

	// Security Features Tests
	Context("when configuring security features", func() {
		It("should create cluster with database encryption when specified", func() {
			config.Spec.DatabaseEncryption = &gkev1.GKEDatabaseEncryption{
				State:   "ENCRYPTED",
				KeyName: "projects/test/locations/us-central1/keyRings/ring/cryptoKeys/key",
			}

			request := NewClusterCreateRequest(config)
			Expect(request.Cluster.DatabaseEncryption).ToNot(BeNil())
			Expect(request.Cluster.DatabaseEncryption.State).To(Equal("ENCRYPTED"))
			Expect(request.Cluster.DatabaseEncryption.KeyName).To(Equal("projects/test/locations/us-central1/keyRings/ring/cryptoKeys/key"))
		})

		It("should not set database encryption when not specified", func() {
			config.Spec.DatabaseEncryption = nil

			request := NewClusterCreateRequest(config)
			Expect(request.Cluster.DatabaseEncryption).To(BeNil())
		})

		It("should create cluster with binary authorization enabled", func() {
			config.Spec.BinaryAuthorization = &gkev1.GKEBinaryAuthorization{
				Enabled: true,
			}

			request := NewClusterCreateRequest(config)
			Expect(request.Cluster.BinaryAuthorization).ToNot(BeNil())
			Expect(request.Cluster.BinaryAuthorization.Enabled).To(BeTrue())
		})

		It("should create cluster with binary authorization disabled", func() {
			config.Spec.BinaryAuthorization = &gkev1.GKEBinaryAuthorization{
				Enabled: false,
			}

			request := NewClusterCreateRequest(config)
			Expect(request.Cluster.BinaryAuthorization).ToNot(BeNil())
			Expect(request.Cluster.BinaryAuthorization.Enabled).To(BeFalse())
		})

		It("should not set binary authorization when not specified", func() {
			config.Spec.BinaryAuthorization = nil

			request := NewClusterCreateRequest(config)
			Expect(request.Cluster.BinaryAuthorization).To(BeNil())
		})

		It("should create cluster with shielded nodes enabled", func() {
			config.Spec.ShieldedNodes = &gkev1.GKEShieldedNodes{
				Enabled: true,
			}

			request := NewClusterCreateRequest(config)
			Expect(request.Cluster.ShieldedNodes).ToNot(BeNil())
			Expect(request.Cluster.ShieldedNodes.Enabled).To(BeTrue())
		})

		It("should not set shielded nodes when not specified", func() {
			config.Spec.ShieldedNodes = nil

			request := NewClusterCreateRequest(config)
			Expect(request.Cluster.ShieldedNodes).To(BeNil())
		})

		It("should create cluster with workload identity configured", func() {
			config.Spec.WorkloadIdentityConfig = &gkev1.GKEWorkloadIdentityConfig{
				WorkloadPool: "test-project.svc.id.goog",
			}

			request := NewClusterCreateRequest(config)
			Expect(request.Cluster.WorkloadIdentityConfig).ToNot(BeNil())
			Expect(request.Cluster.WorkloadIdentityConfig.WorkloadPool).To(Equal("test-project.svc.id.goog"))
		})

		It("should not set workload identity when not specified", func() {
			config.Spec.WorkloadIdentityConfig = nil

			request := NewClusterCreateRequest(config)
			Expect(request.Cluster.WorkloadIdentityConfig).To(BeNil())
		})

		It("should create cluster with legacy ABAC disabled", func() {
			config.Spec.LegacyAbac = &gkev1.GKELegacyAbac{
				Enabled: false,
			}

			request := NewClusterCreateRequest(config)
			Expect(request.Cluster.LegacyAbac).ToNot(BeNil())
			Expect(request.Cluster.LegacyAbac.Enabled).To(BeFalse())
		})

		It("should not set legacy ABAC when not specified", func() {
			config.Spec.LegacyAbac = nil

			request := NewClusterCreateRequest(config)
			Expect(request.Cluster.LegacyAbac).To(BeNil())
		})

		It("should create cluster with master auth configured without basic auth", func() {
			config.Spec.MasterAuth = &gkev1.GKEMasterAuth{
				Username: "",
				Password: "",
				ClientCertificateConfig: &gkev1.GKEClientCertificateConfig{
					IssueClientCertificate: false,
				},
			}

			request := NewClusterCreateRequest(config)
			Expect(request.Cluster.MasterAuth).ToNot(BeNil())
			Expect(request.Cluster.MasterAuth.Username).To(Equal(""))
			Expect(request.Cluster.MasterAuth.Password).To(Equal(""))
			Expect(request.Cluster.MasterAuth.ClientCertificateConfig).ToNot(BeNil())
			Expect(request.Cluster.MasterAuth.ClientCertificateConfig.IssueClientCertificate).To(BeFalse())
		})

		It("should not set master auth when not specified", func() {
			config.Spec.MasterAuth = nil

			request := NewClusterCreateRequest(config)
			Expect(request.Cluster.MasterAuth).To(BeNil())
		})

		It("should create cluster with intra-node visibility enabled", func() {
			config.Spec.IntraNodeVisibilityConfig = &gkev1.GKEIntraNodeVisibilityConfig{
				Enabled: true,
			}

			request := NewClusterCreateRequest(config)
			Expect(request.Cluster.NetworkConfig).ToNot(BeNil())
			Expect(request.Cluster.NetworkConfig.EnableIntraNodeVisibility).To(BeTrue())
		})

		It("should not enable intra-node visibility when not specified", func() {
			config.Spec.IntraNodeVisibilityConfig = nil

			request := NewClusterCreateRequest(config)
			if request.Cluster.NetworkConfig != nil {
				Expect(request.Cluster.NetworkConfig.EnableIntraNodeVisibility).To(BeFalse())
			}
		})
	})

	Context("when configuring node pool security features", func() {
		BeforeEach(func() {
			// Disable autopilot to allow custom node pools
			config.Spec.AutopilotConfig = &gkev1.GKEAutopilotConfig{
				Enabled: false,
			}
			// Ensure we have a node pool for testing
			config.Spec.NodePools = []gkev1.GKENodePoolConfig{
				{
					Name:              &nodePoolName,
					InitialNodeCount:  &initialNodeCount,
					Version:           &k8sVersion,
					MaxPodsConstraint: &maxPodsConstraint,
					Config:            &gkev1.GKENodeConfig{},
					Autoscaling: &gkev1.GKENodePoolAutoscaling{
						Enabled:      false,
						MinNodeCount: 1,
						MaxNodeCount: 3,
					},
					Management: &gkev1.GKENodePoolManagement{
						AutoRepair:  true,
						AutoUpgrade: true,
					},
				},
			}
		})

		It("should create node pool with shielded instance config", func() {
			config.Spec.NodePools[0].Config.ShieldedInstanceConfig = &gkev1.GKEShieldedInstanceConfig{
				EnableIntegrityMonitoring: true,
				EnableSecureBoot:          true,
			}

			request := NewClusterCreateRequest(config)
			Expect(len(request.Cluster.NodePools)).To(BeNumerically(">", 0))
			nodePool := request.Cluster.NodePools[0]
			Expect(nodePool.Config.ShieldedInstanceConfig).ToNot(BeNil())
			Expect(nodePool.Config.ShieldedInstanceConfig.EnableIntegrityMonitoring).To(BeTrue())
			Expect(nodePool.Config.ShieldedInstanceConfig.EnableSecureBoot).To(BeTrue())
		})

		It("should create node pool with workload metadata config", func() {
			config.Spec.NodePools[0].Config.WorkloadMetadataConfig = &gkev1.GKEWorkloadMetadataConfig{
				Mode: "GKE_METADATA",
			}

			request := NewClusterCreateRequest(config)
			Expect(len(request.Cluster.NodePools)).To(BeNumerically(">", 0))
			nodePool := request.Cluster.NodePools[0]
			Expect(nodePool.Config.WorkloadMetadataConfig).ToNot(BeNil())
			Expect(nodePool.Config.WorkloadMetadataConfig.Mode).To(Equal("GKE_METADATA"))
		})

		It("should create node pool with boot disk KMS key", func() {
			config.Spec.NodePools[0].Config.BootDiskKmsKey = "projects/test/locations/us-central1/keyRings/ring/cryptoKeys/key"

			request := NewClusterCreateRequest(config)
			Expect(len(request.Cluster.NodePools)).To(BeNumerically(">", 0))
			nodePool := request.Cluster.NodePools[0]
			Expect(nodePool.Config.BootDiskKmsKey).To(Equal("projects/test/locations/us-central1/keyRings/ring/cryptoKeys/key"))
		})
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
