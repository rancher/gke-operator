module github.com/rancher/gke-operator

go 1.16

replace k8s.io/client-go => k8s.io/client-go v0.21.2

require (
	github.com/rancher/lasso v0.0.0-20210616224652-fc3ebd901c08
	github.com/rancher/wrangler v0.8.3
	github.com/rancher/wrangler-api v0.6.1-0.20200427172631-a7c2f09b783e
	github.com/sirupsen/logrus v1.6.0
	golang.org/x/oauth2 v0.0.0-20201208152858-08078c50e5b5
	google.golang.org/api v0.40.0
	k8s.io/api v0.21.2
	k8s.io/apimachinery v0.21.2
	k8s.io/client-go v12.0.0+incompatible
)
