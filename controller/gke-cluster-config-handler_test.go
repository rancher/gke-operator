package controller

import (
	"context"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gkeapi "google.golang.org/api/container/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gkev1 "github.com/rancher/gke-operator/pkg/apis/gke.cattle.io/v1"
	"github.com/rancher/gke-operator/pkg/gke"
	"github.com/rancher/gke-operator/pkg/gke/services/mock_services"

	"github.com/rancher/gke-operator/pkg/test"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("createCASecret", func() {
	var (
		handler      *Handler
		gkeConfig    *gkev1.GKEClusterConfig
		caSecret     *corev1.Secret
		clusterState *gkeapi.Cluster
	)

	BeforeEach(func() {
		gkeConfig = &gkev1.GKEClusterConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
			},
			Spec: gkev1.GKEClusterConfigSpec{
				ClusterName: "test",
				ProjectID:   "example-project-name",
			},
		}
		Expect(cl.Create(ctx, gkeConfig)).To(Succeed())

		caSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      gkeConfig.Name,
				Namespace: gkeConfig.Namespace,
			},
		}

		clusterState = &gkeapi.Cluster{
			Name:     "test-cluster",
			Endpoint: "https://test.com",
			MasterAuth: &gkeapi.MasterAuth{
				ClusterCaCertificate: "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCnRlc3QKLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
			},
		}

		handler = &Handler{
			gkeCC:        gkeFactory.Gke().V1().GKEClusterConfig(),
			secrets:      coreFactory.Core().V1().Secret(),
			secretsCache: coreFactory.Core().V1().Secret().Cache(),
		}
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, gkeConfig, caSecret)).To(Succeed())
	})

	It("should create CA secret", func() {
		err := handler.createCASecret(gkeConfig, clusterState)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should return nil if caSecret exist", func() {
		Expect(cl.Create(ctx, caSecret)).To(Succeed())

		err := handler.createCASecret(gkeConfig, clusterState)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should return error if cluster endpoint is empty", func() {
		clusterState.Endpoint = ""

		err := handler.createCASecret(gkeConfig, clusterState)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("has no endpoint"))
	})

	It("should return error if cluster CA doesn't exist", func() {
		clusterState.MasterAuth = nil

		err := handler.createCASecret(gkeConfig, clusterState)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("has no CA"))
	})
})

var _ = Describe("buildUpstreamClusterState", func() {
	var (
		handler      *Handler
		clusterState *gkeapi.Cluster
	)

	BeforeEach(func() {
		clusterState = &gkeapi.Cluster{
			Name:                 "test-cluster",
			CurrentMasterVersion: "1.28.10-gke.1089000",
			Zone:                 "test-east1",
			Locations:            []string{"test-east1-a", "test-east1-b", "test-east1-c"},
			Endpoint:             "https://test.com",
			MasterAuth: &gkeapi.MasterAuth{
				ClusterCaCertificate: "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCnRlc3QKLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
			},
			EnableKubernetesAlpha: true,
			ClusterIpv4Cidr:       "10.10.10.0/24",
			LoggingService:        "logging.googleapis.com",
			MonitoringService:     "monitoring.googleapis.com",
			Network:               "test-network",
			Subnetwork:            "test-subnetwork",
			IpAllocationPolicy:    &gkeapi.IPAllocationPolicy{},
			MasterAuthorizedNetworksConfig: &gkeapi.MasterAuthorizedNetworksConfig{
				Enabled: false,
			},
			ResourceLabels: map[string]string{
				"label1": "test1",
				"label2": "test2",
				"label3": "test3",
			},
			NetworkPolicy: &gkeapi.NetworkPolicy{
				Enabled: true,
			},
			PrivateClusterConfig: &gkeapi.PrivateClusterConfig{
				EnablePrivateEndpoint: true,
				EnablePrivateNodes:    true,
				MasterIpv4CidrBlock:   "172.16.0.0/28",
			},
			AddonsConfig: &gkeapi.AddonsConfig{
				HttpLoadBalancing: &gkeapi.HttpLoadBalancing{
					Disabled: false,
				},
				HorizontalPodAutoscaling: &gkeapi.HorizontalPodAutoscaling{
					Disabled: false,
				},
				NetworkPolicyConfig: &gkeapi.NetworkPolicyConfig{
					Disabled: false,
				},
			},
			NodePools: []*gkeapi.NodePool{
				{
					Name:             "np-tes1",
					InitialNodeCount: 1,
					Version:          "1.28.10-gke.1089000",
					MaxPodsConstraint: &gkeapi.MaxPodsConstraint{
						MaxPodsPerNode: 110,
					},
					Config: &gkeapi.NodeConfig{
						MachineType: "n1-standard-1",
						DiskSizeGb:  100,
						DiskType:    "pd-standard",
						ImageType:   "COS",
						Preemptible: false,
					},
					Autoscaling: &gkeapi.NodePoolAutoscaling{
						Enabled:      true,
						MinNodeCount: 3,
						MaxNodeCount: 5,
					},
					Management: &gkeapi.NodeManagement{
						AutoRepair:  true,
						AutoUpgrade: true,
					},
				},
			},
		}

		handler = &Handler{
			gkeCC:        gkeFactory.Gke().V1().GKEClusterConfig(),
			secrets:      coreFactory.Core().V1().Secret(),
			secretsCache: coreFactory.Core().V1().Secret().Cache(),
		}
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl)).To(Succeed())
	})

	It("should build upstream cluster state", func() {
		upstreamSpec, err := handler.buildUpstreamClusterState(clusterState)
		Expect(err).ToNot(HaveOccurred())
		Expect(upstreamSpec).ToNot(BeNil())

		Expect(*upstreamSpec.KubernetesVersion).To(Equal(clusterState.CurrentMasterVersion))
		Expect(*upstreamSpec.EnableKubernetesAlpha).To(Equal(clusterState.EnableKubernetesAlpha))
		Expect(*upstreamSpec.ClusterIpv4CidrBlock).To(Equal(clusterState.ClusterIpv4Cidr))
		Expect(*upstreamSpec.Network).To(Equal(clusterState.Network))
		Expect(*upstreamSpec.Subnetwork).To(Equal(clusterState.Subnetwork))
		Expect(*upstreamSpec.LoggingService).To(Equal(clusterState.LoggingService))
		Expect(*upstreamSpec.MonitoringService).To(Equal(clusterState.MonitoringService))
		Expect(upstreamSpec.MasterAuthorizedNetworksConfig.Enabled).To(Equal(clusterState.MasterAuthorizedNetworksConfig.Enabled))
		Expect(upstreamSpec.Labels["label1"]).To(Equal(clusterState.ResourceLabels["label1"]))
		Expect(upstreamSpec.Labels["label2"]).To(Equal(clusterState.ResourceLabels["label2"]))
		Expect(upstreamSpec.Labels["label3"]).To(Equal(clusterState.ResourceLabels["label3"]))
		Expect(*upstreamSpec.NetworkPolicyEnabled).To(Equal(clusterState.NetworkPolicy.Enabled))
		Expect(upstreamSpec.PrivateClusterConfig.EnablePrivateEndpoint).To(Equal(clusterState.PrivateClusterConfig.EnablePrivateEndpoint))
		Expect(upstreamSpec.PrivateClusterConfig.EnablePrivateNodes).To(Equal(clusterState.PrivateClusterConfig.EnablePrivateNodes))
		Expect(upstreamSpec.PrivateClusterConfig.MasterIpv4CidrBlock).To(Equal(clusterState.PrivateClusterConfig.MasterIpv4CidrBlock))
		Expect(upstreamSpec.ClusterAddons.HTTPLoadBalancing).To(Equal(!clusterState.AddonsConfig.HttpLoadBalancing.Disabled))
		Expect(upstreamSpec.ClusterAddons.HorizontalPodAutoscaling).To(Equal(!clusterState.AddonsConfig.HorizontalPodAutoscaling.Disabled))
		Expect(upstreamSpec.ClusterAddons.NetworkPolicyConfig).To(Equal(!clusterState.AddonsConfig.NetworkPolicyConfig.Disabled))
		Expect(upstreamSpec.NodePools).To(HaveLen(1))
		Expect(*upstreamSpec.NodePools[0].Name).To(Equal(clusterState.NodePools[0].Name))
		Expect(*upstreamSpec.NodePools[0].InitialNodeCount).To(Equal(clusterState.NodePools[0].InitialNodeCount))
		Expect(*upstreamSpec.NodePools[0].Version).To(Equal(clusterState.NodePools[0].Version))
		Expect(*upstreamSpec.NodePools[0].MaxPodsConstraint).To(Equal(clusterState.NodePools[0].MaxPodsConstraint.MaxPodsPerNode))
		Expect(upstreamSpec.NodePools[0].Config.MachineType).To(Equal(clusterState.NodePools[0].Config.MachineType))
		Expect(upstreamSpec.NodePools[0].Config.DiskSizeGb).To(Equal(clusterState.NodePools[0].Config.DiskSizeGb))
		Expect(upstreamSpec.NodePools[0].Config.DiskType).To(Equal(clusterState.NodePools[0].Config.DiskType))
		Expect(upstreamSpec.NodePools[0].Config.ImageType).To(Equal(clusterState.NodePools[0].Config.ImageType))
		Expect(upstreamSpec.NodePools[0].Config.Preemptible).To(Equal(clusterState.NodePools[0].Config.Preemptible))
		Expect(upstreamSpec.NodePools[0].Autoscaling.Enabled).To(Equal(clusterState.NodePools[0].Autoscaling.Enabled))
		Expect(upstreamSpec.NodePools[0].Autoscaling.MinNodeCount).To(Equal(clusterState.NodePools[0].Autoscaling.MinNodeCount))
		Expect(upstreamSpec.NodePools[0].Autoscaling.MaxNodeCount).To(Equal(clusterState.NodePools[0].Autoscaling.MaxNodeCount))
		Expect(upstreamSpec.NodePools[0].Management.AutoRepair).To(Equal(clusterState.NodePools[0].Management.AutoRepair))
		Expect(upstreamSpec.NodePools[0].Management.AutoUpgrade).To(Equal(clusterState.NodePools[0].Management.AutoUpgrade))
	})
})

const authTestJson = `
{
	"type": "service_account",
	"project_id": "test-project",
	"private_key_id": "1234567890123456",
	"private_key": "-----BEGIN PRIVATE KEY-----\nMIICWwIBAAKBgQCjq3PA6Un07pRRcIJVaKSdj2g2QOWWkjjfAdRqxfvVAfBZtryP\nmUyo/JxBAfRpHc9an9FkOJNH9MPwOQdbxmU2S5zuAsbRmpYXd5q7tIqO++XBTQBf\ng4THdSvHJrzGx9uYzOJfMATiI8mif5DZkblxQMgdzBwqutf+VvzkTbk3nwIDAQAB\nAoGAEvcJAK+HnFQQ56br002+1WsKnk7Cy8HByUWDAaRTXAlPenXMP695zJMI4BeD\n5LJJlqyyLLTJjCr2kV1qVt4UWBjJ4q7qfI0tVwHcK5laM034z8XNbTgknqPxVJaU\nW90B3T+vQIDfu7PDZjxsoKE1BioXSxyL26tR4NMKCrxz0dECQQDWW0lWalFUpHDI\ntG/JOpTX9DFCnrvzqZ6BP0LBn2Ps0XuMQkfiKMOEbxKm5yeB2ULO2giFY+OVdxKO\nXmEQwLgFAkEAw3dUJRL2Rb7PGIJ2nPmnvs6lmEN3jNPaHjLArEe2Rw7kf6XQxt+s\nbVtwO0WJ86NA4Yf3WLBQnN1xpvJTSOK2UwJAYQcHJj+PuvGIP8E1DHAg6bOWDKLP\nTtcLcVOSQxSD5bFY7D8gTKXJAoxIdBYT0vnl/L3Ct6ZkYMZ6NslPxIaHhQJAGZQD\n7tYMZBQUBaEM5H3G9bEU+lfZzRPr9wetLt4zfBj2zb1lFKEwbx8IELmI09kJJHom\nY/Sul9hihvYu79q7AQJAWgx1VbozkvAbedRrd5GCYQH7yBVnQuWA49N4su494qd4\nvLotJ+520U/9JcdtiL+Q3EdjO5Y60eJLX/PIBvZcig==\n-----END PRIVATE KEY-----\n",
	"client_email": "test",
	"client_id": "test",
	"auth_uri": "https://www.example.com",
	"token_uri": "https://www.example.com",
	"auth_provider_x509_cert_url": "https://www.example.com",
	"client_x509_cert_url": "https://www.example.com",
	"universe_domain": "example.com"
}`

var _ = Describe("importCluster", func() {
	var (
		handler             *Handler
		mockController      *gomock.Controller
		gkeServiceMock      *mock_services.MockGKEClusterService
		gkeConfig           *gkev1.GKEClusterConfig
		credentialSecret    *corev1.Secret
		caSecret            *corev1.Secret
		importTestNamespace *corev1.Namespace
		clusterState        *gkeapi.Cluster
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		gkeServiceMock = mock_services.NewMockGKEClusterService(mockController)

		clusterState = &gkeapi.Cluster{
			Name:                 "test-cluster",
			CurrentMasterVersion: "1.28.10-gke.1089000",
			Zone:                 "test-east1",
			Locations:            []string{"test-east1-a", "test-east1-b", "test-east1-c"},
			Endpoint:             "https://test.com",
			MasterAuth: &gkeapi.MasterAuth{
				ClusterCaCertificate: "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCnRlc3QKLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
			},
			EnableKubernetesAlpha: true,
			ClusterIpv4Cidr:       "10.10.10.0/24",
			LoggingService:        "logging.googleapis.com",
			MonitoringService:     "monitoring.googleapis.com",
			Network:               "test-network",
			Subnetwork:            "test-subnetwork",
			IpAllocationPolicy:    &gkeapi.IPAllocationPolicy{},
			MasterAuthorizedNetworksConfig: &gkeapi.MasterAuthorizedNetworksConfig{
				Enabled: false,
			},
			ResourceLabels: map[string]string{
				"label1": "test1",
				"label2": "test2",
				"label3": "test3",
			},
			NetworkPolicy: &gkeapi.NetworkPolicy{
				Enabled: true,
			},
			PrivateClusterConfig: &gkeapi.PrivateClusterConfig{
				EnablePrivateEndpoint: true,
				EnablePrivateNodes:    true,
				MasterIpv4CidrBlock:   "172.16.0.0/28",
			},
			AddonsConfig: &gkeapi.AddonsConfig{
				HttpLoadBalancing: &gkeapi.HttpLoadBalancing{
					Disabled: false,
				},
				HorizontalPodAutoscaling: &gkeapi.HorizontalPodAutoscaling{
					Disabled: false,
				},
				NetworkPolicyConfig: &gkeapi.NetworkPolicyConfig{
					Disabled: false,
				},
			},
			NodePools: []*gkeapi.NodePool{
				{
					Name:             "np-tes1",
					InitialNodeCount: 1,
					Version:          "1.28.10-gke.1089000",
					MaxPodsConstraint: &gkeapi.MaxPodsConstraint{
						MaxPodsPerNode: 110,
					},
					Config: &gkeapi.NodeConfig{
						MachineType: "n1-standard-1",
						DiskSizeGb:  100,
						DiskType:    "pd-standard",
						ImageType:   "COS",
						Preemptible: false,
					},
					Autoscaling: &gkeapi.NodePoolAutoscaling{
						Enabled:      true,
						MinNodeCount: 3,
						MaxNodeCount: 5,
					},
					Management: &gkeapi.NodeManagement{
						AutoRepair:  true,
						AutoUpgrade: true,
					},
				},
			},
		}

		importTestNamespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "import-test-namespace",
			},
			Spec: corev1.NamespaceSpec{},
		}
		cl.Create(ctx, importTestNamespace)

		credentialSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-secret",
				Namespace: importTestNamespace.Name,
			},
			Data: map[string][]byte{
				"googlecredentialConfig-authEncodedJson": []byte(authTestJson),
			},
		}
		Expect(cl.Create(ctx, credentialSecret)).To(Succeed())
		cl.Get(ctx, client.ObjectKeyFromObject(credentialSecret), credentialSecret)

		gkeConfig = &gkev1.GKEClusterConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: importTestNamespace.Name,
			},
			Spec: gkev1.GKEClusterConfigSpec{
				ClusterName:            "test-cluster",
				ProjectID:              "example-project-name",
				GoogleCredentialSecret: credentialSecret.Namespace + ":" + credentialSecret.Name,
				Imported:               true,
			},
		}
		Expect(cl.Create(ctx, gkeConfig)).To(Succeed())

		caSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      gkeConfig.Name,
				Namespace: gkeConfig.Namespace,
			},
		}

		handler = &Handler{
			gkeCC:        gkeFactory.Gke().V1().GKEClusterConfig(),
			secrets:      coreFactory.Core().V1().Secret(),
			secretsCache: coreFactory.Core().V1().Secret().Cache(),
			gkeClient:    gkeServiceMock,
			gkeClientCtx: context.Background(),
		}
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, credentialSecret, gkeConfig, caSecret)).To(Succeed())
	})

	It("should import cluster and update status", func() {
		gkeServiceMock.EXPECT().
			ClusterGet(
				context.Background(),
				gke.ClusterRRN(gkeConfig.Spec.ProjectID, gke.Location(gkeConfig.Spec.Region, gkeConfig.Spec.Zone),
					gkeConfig.Spec.ClusterName)).
			Return(clusterState, nil)

		gotGKEConfig, err := handler.importCluster(gkeConfig)
		Expect(err).ToNot(HaveOccurred())
		Expect(gotGKEConfig.Status.Phase).To(Equal(gkeConfigActivePhase))
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(caSecret), caSecret)).To(Succeed())
		Expect(caSecret.OwnerReferences).To(HaveLen(1))
		Expect(caSecret.OwnerReferences[0].Name).To(Equal(gotGKEConfig.Name))
		Expect(caSecret.OwnerReferences[0].Kind).To(Equal(gkeClusterConfigKind))
		Expect(caSecret.OwnerReferences[0].APIVersion).To(Equal(gkev1.SchemeGroupVersion.String()))
		Expect(caSecret.Data["endpoint"]).To(Equal([]byte("https://test.com")))
		Expect(caSecret.Data["ca"]).To(Equal([]byte("LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCnRlc3QKLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=")))

	})

	It("should return error if cluster doesn't exist", func() {
		gkeServiceMock.EXPECT().
			ClusterGet(
				context.Background(),
				gke.ClusterRRN(gkeConfig.Spec.ProjectID, gke.Location(gkeConfig.Spec.Region, gkeConfig.Spec.Zone),
					gkeConfig.Spec.ClusterName)).
			Return(&gkeapi.Cluster{}, nil)

		gotGKEConfig, err := handler.importCluster(gkeConfig)
		Expect(err).To(HaveOccurred())
		Expect(gotGKEConfig).NotTo(BeNil())
	})
})

var _ = Describe("createCluster", func() {
	var (
		handler             *Handler
		mockController      *gomock.Controller
		gkeServiceMock      *mock_services.MockGKEClusterService
		gkeConfig           *gkev1.GKEClusterConfig
		credentialSecret    *corev1.Secret
		caSecret            *corev1.Secret
		createTestNamespace *corev1.Namespace
		clusterState        *gkeapi.Cluster
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		gkeServiceMock = mock_services.NewMockGKEClusterService(mockController)

		clusterState = &gkeapi.Cluster{
			Name:                 "test-cluster",
			CurrentMasterVersion: "1.28.10-gke.1089000",
			Zone:                 "test-east1",
			Locations:            []string{"test-east1-a", "test-east1-b", "test-east1-c"},
			Endpoint:             "https://test.com",
			MasterAuth: &gkeapi.MasterAuth{
				ClusterCaCertificate: "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCnRlc3QKLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=",
			},
			EnableKubernetesAlpha: true,
			ClusterIpv4Cidr:       "10.10.10.0/24",
			LoggingService:        "logging.googleapis.com",
			MonitoringService:     "monitoring.googleapis.com",
			Network:               "test-network",
			Subnetwork:            "test-subnetwork",
			IpAllocationPolicy:    &gkeapi.IPAllocationPolicy{},
			MasterAuthorizedNetworksConfig: &gkeapi.MasterAuthorizedNetworksConfig{
				Enabled: false,
			},
			ResourceLabels: map[string]string{
				"label1": "test1",
				"label2": "test2",
				"label3": "test3",
			},
			NetworkPolicy: &gkeapi.NetworkPolicy{
				Enabled: true,
			},
			PrivateClusterConfig: &gkeapi.PrivateClusterConfig{
				EnablePrivateEndpoint: true,
				EnablePrivateNodes:    true,
				MasterIpv4CidrBlock:   "172.16.0.0/28",
			},
			AddonsConfig: &gkeapi.AddonsConfig{
				HttpLoadBalancing: &gkeapi.HttpLoadBalancing{
					Disabled: false,
				},
				HorizontalPodAutoscaling: &gkeapi.HorizontalPodAutoscaling{
					Disabled: false,
				},
				NetworkPolicyConfig: &gkeapi.NetworkPolicyConfig{
					Disabled: false,
				},
			},
			NodePools: []*gkeapi.NodePool{
				{
					Name:             "np-test1",
					InitialNodeCount: 1,
					Version:          "1.28.10-gke.1089000",
					MaxPodsConstraint: &gkeapi.MaxPodsConstraint{
						MaxPodsPerNode: 110,
					},
					Config: &gkeapi.NodeConfig{
						MachineType: "n1-standard-1",
						DiskSizeGb:  100,
						DiskType:    "pd-standard",
						ImageType:   "COS",
						Preemptible: false,
					},
					Autoscaling: &gkeapi.NodePoolAutoscaling{
						Enabled:      true,
						MinNodeCount: 3,
						MaxNodeCount: 5,
					},
					Management: &gkeapi.NodeManagement{
						AutoRepair:  true,
						AutoUpgrade: true,
					},
				},
			},
		}

		createTestNamespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "create-test-namespace",
			},
			Spec: corev1.NamespaceSpec{},
		}
		cl.Create(ctx, createTestNamespace)

		credentialSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-secret",
				Namespace: createTestNamespace.Name,
			},
			Data: map[string][]byte{
				"googlecredentialConfig-authEncodedJson": []byte(authTestJson),
			},
		}
		Expect(cl.Create(ctx, credentialSecret)).To(Succeed())
		cl.Get(ctx, client.ObjectKeyFromObject(credentialSecret), credentialSecret)

		k8sVersion := "1.25.12-gke.200"
		clusterIpv4Cidr := "10.42.0.0/16"
		networkName := "test-network"
		subnetworkName := "test-subnetwork"
		emptyString := ""
		//boolTrue := true
		boolFalse := false
		nodePoolName := "test-node-pool"
		initialNodeCount := int64(3)
		maxPodsConstraint := int64(110)

		gkeConfig = &gkev1.GKEClusterConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: createTestNamespace.Name,
			},
			Spec: gkev1.GKEClusterConfigSpec{
				ClusterName:           "test-cluster",
				Description:           "test cluster",
				ProjectID:             "example-project-name",
				Region:                "test-east1",
				Labels:                map[string]string{"test": "test"},
				KubernetesVersion:     &k8sVersion,
				LoggingService:        &emptyString,
				MonitoringService:     &emptyString,
				EnableKubernetesAlpha: &boolFalse,
				ClusterIpv4CidrBlock:  &clusterIpv4Cidr,
				IPAllocationPolicy: &gkev1.GKEIPAllocationPolicy{
					UseIPAliases: true,
				},
				NodePools: []gkev1.GKENodePoolConfig{
					{
						Name: &nodePoolName,
						Autoscaling: &gkev1.GKENodePoolAutoscaling{
							Enabled: false,
						},
						Config:            &gkev1.GKENodeConfig{},
						InitialNodeCount:  &initialNodeCount,
						MaxPodsConstraint: &maxPodsConstraint,
						Version:           &k8sVersion,
						Management: &gkev1.GKENodePoolManagement{
							AutoRepair:  true,
							AutoUpgrade: true,
						},
					},
				},
				ClusterAddons: &gkev1.GKEClusterAddons{
					HTTPLoadBalancing:        true,
					NetworkPolicyConfig:      false,
					HorizontalPodAutoscaling: true,
				},
				NetworkPolicyEnabled: &boolFalse,
				Network:              &networkName,
				Subnetwork:           &subnetworkName,
				PrivateClusterConfig: &gkev1.GKEPrivateClusterConfig{
					EnablePrivateEndpoint: false,
					EnablePrivateNodes:    false,
				},
				MasterAuthorizedNetworksConfig: &gkev1.GKEMasterAuthorizedNetworksConfig{
					Enabled: false,
				},
				Locations:              []string{},
				MaintenanceWindow:      &emptyString,
				GoogleCredentialSecret: credentialSecret.Namespace + ":" + credentialSecret.Name,
				Imported:               false,
			},
		}
		Expect(cl.Create(ctx, gkeConfig)).To(Succeed())

		caSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      gkeConfig.Name,
				Namespace: gkeConfig.Namespace,
			},
		}

		handler = &Handler{
			gkeCC:        gkeFactory.Gke().V1().GKEClusterConfig(),
			secrets:      coreFactory.Core().V1().Secret(),
			secretsCache: coreFactory.Core().V1().Secret().Cache(),
			gkeClient:    gkeServiceMock,
			gkeClientCtx: context.Background(),
		}
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, credentialSecret, gkeConfig, caSecret)).To(Succeed())
	})

	It("should create cluster and update status", func() {
		createClusterRequest := gke.NewClusterCreateRequest(gkeConfig)
		gkeServiceMock.EXPECT().
			ClusterCreate(
				context.Background(),
				gke.LocationRRN(gkeConfig.Spec.ProjectID, gke.Location(gkeConfig.Spec.Region, gkeConfig.Spec.Zone)),
				createClusterRequest).
			Return(&gkeapi.Operation{}, nil)

		gkeServiceMock.EXPECT().
			ClusterList(
				context.Background(),
				gke.LocationRRN(gkeConfig.Spec.ProjectID, gke.Location(gkeConfig.Spec.Region, gkeConfig.Spec.Zone))).
			Return(&gkeapi.ListClustersResponse{}, nil)

		gotGKEConfig, err := handler.create(gkeConfig)
		Expect(err).ToNot(HaveOccurred())
		Expect(gotGKEConfig.Status.Phase).To(Equal(gkeConfigCreatingPhase))
	})

	It("should return error if cluster already exist", func() {
		ctx := context.Background()

		gkeServiceMock.EXPECT().
			ClusterList(
				ctx,
				gke.LocationRRN(gkeConfig.Spec.ProjectID, gke.Location(gkeConfig.Spec.Region, gkeConfig.Spec.Zone))).
			Return(&gkeapi.ListClustersResponse{
				Clusters: []*gkeapi.Cluster{clusterState},
			}, nil)

		gotGKEConfig, err := handler.create(gkeConfig)
		Expect(err).To(HaveOccurred())
		Expect(gotGKEConfig).NotTo(BeNil())
	})

	It("should create a cluster when no service account has been set", func() {
		ctx := context.Background()
		gkeConfig.Spec.NodePools[0].Config.ServiceAccount = ""

		gkeServiceMock.EXPECT().ClusterList(
			ctx,
			gke.LocationRRN(gkeConfig.Spec.ProjectID, gke.Location(gkeConfig.Spec.Region, gkeConfig.Spec.Zone))).
			Return(&gkeapi.ListClustersResponse{}, nil)

		ccr := gke.NewClusterCreateRequest(gkeConfig)
		Expect(ccr.Cluster.NodePools[0].Config.ServiceAccount).To(Equal(""))
		gkeServiceMock.EXPECT().ClusterCreate(
			ctx,
			gke.LocationRRN(gkeConfig.Spec.ProjectID, gke.Location(gkeConfig.Spec.Region, gkeConfig.Spec.Zone)),
			ccr,
		)

		gotGKEConfig, err := handler.create(gkeConfig)
		Expect(err).ToNot(HaveOccurred())
		Expect(gotGKEConfig).NotTo(BeNil())
	})

	It("should create a cluster with the default service acount when the default service account has been set", func() {
		ctx := context.Background()
		gkeConfig.Spec.NodePools[0].Config.ServiceAccount = "default"

		gkeServiceMock.EXPECT().ClusterList(
			ctx,
			gke.LocationRRN(gkeConfig.Spec.ProjectID, gke.Location(gkeConfig.Spec.Region, gkeConfig.Spec.Zone))).
			Return(&gkeapi.ListClustersResponse{}, nil)

		ccr := gke.NewClusterCreateRequest(gkeConfig)
		Expect(ccr.Cluster.NodePools[0].Config.ServiceAccount).To(Equal("default"))
		gkeServiceMock.EXPECT().ClusterCreate(
			ctx,
			gke.LocationRRN(gkeConfig.Spec.ProjectID, gke.Location(gkeConfig.Spec.Region, gkeConfig.Spec.Zone)),
			ccr,
		)

		gotGKEConfig, err := handler.create(gkeConfig)
		Expect(err).ToNot(HaveOccurred())
		Expect(gotGKEConfig).NotTo(BeNil())
	})

	It("should create a cluster with the specified service acount when a service account email address has been set", func() {
		ctx := context.Background()
		gkeConfig.Spec.NodePools[0].Config.ServiceAccount = "test@example-project-name.iam.gserviceaccount.com"

		gkeServiceMock.EXPECT().ClusterList(
			ctx,
			gke.LocationRRN(gkeConfig.Spec.ProjectID, gke.Location(gkeConfig.Spec.Region, gkeConfig.Spec.Zone))).
			Return(&gkeapi.ListClustersResponse{}, nil)

		ccr := gke.NewClusterCreateRequest(gkeConfig)
		Expect(ccr.Cluster.NodePools[0].Config.ServiceAccount).To(Equal("test@example-project-name.iam.gserviceaccount.com"))
		gkeServiceMock.EXPECT().ClusterCreate(
			ctx,
			gke.LocationRRN(gkeConfig.Spec.ProjectID, gke.Location(gkeConfig.Spec.Region, gkeConfig.Spec.Zone)),
			ccr,
		)

		gotGKEConfig, err := handler.create(gkeConfig)
		Expect(err).ToNot(HaveOccurred())
		Expect(gotGKEConfig).NotTo(BeNil())
	})

	It("should create a cluster with the specified service acount assigned to a node pool when a service account has been set for that node pool", func() {
		ctx := context.Background()

		// Add a second node pool to the spec and set its service account
		npName := "np-test2"
		npInitialNodeCount := int64(1)
		npMaxPodsConstraint := int64(110)
		npVersion := "1.30.0"
		gkeConfig.Spec.NodePools = append(gkeConfig.Spec.NodePools, gkev1.GKENodePoolConfig{
			Name:              &npName,
			Autoscaling:       &gkev1.GKENodePoolAutoscaling{},
			InitialNodeCount:  &npInitialNodeCount,
			MaxPodsConstraint: &npMaxPodsConstraint,
			Config: &gkev1.GKENodeConfig{
				ServiceAccount: "test@example-project-name.iam.gserviceaccount.com",
			},
			Version:    &npVersion,
			Management: &gkev1.GKENodePoolManagement{},
		})

		gkeServiceMock.EXPECT().ClusterList(
			ctx,
			gke.LocationRRN(gkeConfig.Spec.ProjectID, gke.Location(gkeConfig.Spec.Region, gkeConfig.Spec.Zone))).
			Return(&gkeapi.ListClustersResponse{}, nil)

		ccr := gke.NewClusterCreateRequest(gkeConfig)
		Expect(ccr.Cluster.NodePools[0].Config.ServiceAccount).To(Equal(""))
		Expect(ccr.Cluster.NodePools[1].Config.ServiceAccount).To(Equal("test@example-project-name.iam.gserviceaccount.com"))
		gkeServiceMock.EXPECT().ClusterCreate(
			ctx,
			gke.LocationRRN(gkeConfig.Spec.ProjectID, gke.Location(gkeConfig.Spec.Region, gkeConfig.Spec.Zone)),
			ccr,
		)

		gotGKEConfig, err := handler.create(gkeConfig)
		Expect(err).ToNot(HaveOccurred())
		Expect(gotGKEConfig).NotTo(BeNil())
	})

	It("should not create a cluster if the service account email address is not valid", func() {
		gkeConfig.Spec.NodePools[0].Config.ServiceAccount = "not-a-valid-email-address"

		gkeServiceMock.EXPECT().ClusterList(
			ctx,
			gke.LocationRRN(gkeConfig.Spec.ProjectID, gke.Location(gkeConfig.Spec.Region, gkeConfig.Spec.Zone))).
			Return(&gkeapi.ListClustersResponse{}, nil)

		_, err := handler.create(gkeConfig)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("field [serviceAccount] must either be an empty string, 'default' or set to a valid email address for nodepool [test-node-pool] in non-nil cluster [test-cluster]"))
	})
})
