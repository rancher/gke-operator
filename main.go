//go:generate go run pkg/codegen/cleanup/main.go
//go:generate go run pkg/codegen/main.go

package main

import (
	"context"
	"flag"
	"os"

	"github.com/rancher/gke-operator/controller"
	gkev1 "github.com/rancher/gke-operator/pkg/generated/controllers/gke.cattle.io"
	core3 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core"
	"github.com/rancher/wrangler/v3/pkg/kubeconfig"
	"github.com/rancher/wrangler/v3/pkg/leader"
	"github.com/rancher/wrangler/v3/pkg/signals"
	"github.com/rancher/wrangler/v3/pkg/start"
	"github.com/sirupsen/logrus"

	"k8s.io/client-go/kubernetes"
)

var (
	masterURL      string
	kubeconfigFile string
	debug          bool
	leaderElection bool
)

func init() {
	flag.StringVar(&kubeconfigFile, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.BoolVar(&debug, "debug", false, "Enable debug logs.")
	flag.BoolVar(&leaderElection, "leader-election", true, "Enable leader election for controller manager.")
	flag.Parse()
}

func main() {
	if debug {
		logrus.SetLevel(logrus.DebugLevel)
		logrus.Debugf("Loglevel set to [%v]", logrus.DebugLevel)
	}

	// set up signals so we handle the first shutdown signal gracefully
	ctx := signals.SetupSignalContext()

	// This will load the kubeconfig file in a style the same as kubectl
	cfg, err := kubeconfig.GetNonInteractiveClientConfig(kubeconfigFile).ClientConfig()
	if err != nil {
		logrus.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	// core
	core, err := core3.NewFactoryFromConfig(cfg)
	if err != nil {
		logrus.Fatalf("Error building core factory: %s", err.Error())
	}

	// Generated sample controller
	gke, err := gkev1.NewFactoryFromConfig(cfg)
	if err != nil {
		logrus.Fatalf("Error building gke factory: %s", err.Error())
	}

	if leaderElection {
		// Create Kubernetes client for leader election
		clientset, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			logrus.Fatalf("Error creating kubernetes client: %s", err.Error())
		}

		// Get namespace for leader election
		namespace := os.Getenv("POD_NAMESPACE")
		if namespace == "" {
			namespace = "cattle-system"
		}

		// Use leader election to ensure only one instance runs controllers
		leaderManager := leader.NewManager(namespace, "gke-operator-leader", clientset)
		leaderManager.OnLeader(func(ctx context.Context) error {
			logrus.Info("Became leader, starting controllers")
			
			// Register controllers only when we become leader
			controller.Register(ctx,
				core.Core().V1().Secret(),
				gke.Gke().V1().GKEClusterConfig())

			// Start all the controllers
			return start.All(ctx, 3, gke, core)
		})

		leaderManager.Start(ctx)
	} else {
		// Run without leader election (for development or single instance)
		logrus.Info("Running without leader election")

		// The typical pattern is to build all your controller/clients then just pass to each handler
		// the bare minimum of what they need.  This will eventually help with writing tests.  So
		// don't pass in something like kubeClient, apps, or sample
		controller.Register(ctx,
			core.Core().V1().Secret(),
			gke.Gke().V1().GKEClusterConfig())

		// Start all the controllers
		if err := start.All(ctx, 3, gke, core); err != nil {
			logrus.Fatalf("Error starting: %s", err.Error())
		}
	}

	<-ctx.Done()
}
