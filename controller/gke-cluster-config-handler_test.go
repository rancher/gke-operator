package controller

import (
	"context"
	"fmt"

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
			CurrentMasterVersion: "1.25.13-gke.200",
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
					Version:          "1.25.13-gke.200",
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
		handler          *Handler
		mockController   *gomock.Controller
		gkeServiceMock   *mock_services.MockGKEClusterService
		gkeConfig        *gkev1.GKEClusterConfig
		credentialSecret *corev1.Secret
		caSecret         *corev1.Secret
		testNamespace    *corev1.Namespace
		clusterState     *gkeapi.Cluster
	)

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		gkeServiceMock = mock_services.NewMockGKEClusterService(mockController)

		clusterState = &gkeapi.Cluster{
			Name:                 "test-cluster",
			CurrentMasterVersion: "1.25.13-gke.200",
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
					Version:          "1.25.13-gke.200",
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

		testNamespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-namespace",
			},
			Spec: corev1.NamespaceSpec{},
		}
		Expect(cl.Create(ctx, testNamespace)).To(Succeed())

		credentialSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-secret",
				Namespace: testNamespace.Name,
			},
			Data: map[string][]byte{
				"googlecredentialConfig-authEncodedJson": []byte(authTestJson),
			},
		}
		Expect(cl.Create(ctx, credentialSecret)).To(Succeed())
		cl.Get(ctx, client.ObjectKeyFromObject(credentialSecret), credentialSecret)
		fmt.Printf("TESTO %v", credentialSecret.Data["googlecredentialConfig-authEncodedJson"])

		gkeConfig = &gkev1.GKEClusterConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: testNamespace.Name,
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

		fmt.Println("TEST1")
		gotGKEConfig, err := handler.importCluster(gkeConfig)
		fmt.Println("TEST2")
		Expect(err).ToNot(HaveOccurred())
		fmt.Println("TEST3")
		Expect(gotGKEConfig.Status.Phase).To(Equal(gkeConfigActivePhase))
		fmt.Println("TEST4")
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(caSecret), caSecret)).To(Succeed())
		fmt.Println("TEST5")
		Expect(caSecret.OwnerReferences).To(HaveLen(1))
		Expect(caSecret.OwnerReferences[0].Name).To(Equal(gotGKEConfig.Name))
		Expect(caSecret.OwnerReferences[0].Kind).To(Equal(gkeClusterConfigKind))
		Expect(caSecret.OwnerReferences[0].APIVersion).To(Equal(gkev1.SchemeGroupVersion.String()))
		Expect(caSecret.Data["endpoint"]).To(Equal([]byte("https://test.com")))
		Expect(caSecret.Data["ca"]).To(Equal([]byte("LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCnRlc3QKLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=")))

	})

	/*
		It("should return error if something fails", func() {
			gotGKEConfig, err := handler.importCluster(gkeConfig)
			Expect(err).To(HaveOccurred())
			Expect(gotGKEConfig).NotTo(BeNil())
		})
	*/

})
