package controller

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gkev1 "github.com/rancher/gke-operator/pkg/generated/controllers/gke.cattle.io"
	"github.com/rancher/gke-operator/pkg/test"
	"github.com/rancher/wrangler/pkg/generated/controllers/core"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	testEnv     *envtest.Environment
	cfg         *rest.Config
	cl          client.Client
	coreFactory *core.Factory
	gkeFactory  *gkev1.Factory

	ctx = context.Background()
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GKE Operator Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	var err error
	testEnv = &envtest.Environment{}
	cfg, cl, err = test.StartEnvTest(testEnv)
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())
	Expect(cl).NotTo(BeNil())

	coreFactory, err = core.NewFactoryFromConfig(cfg)
	Expect(err).NotTo(HaveOccurred())
	Expect(coreFactory).NotTo(BeNil())

	gkeFactory, err = gkev1.NewFactoryFromConfig(cfg)
	Expect(err).NotTo(HaveOccurred())
	Expect(gkeFactory).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	Expect(test.StopEnvTest(testEnv)).To(Succeed())
})
