module github.com/rancher/gke-operator

go 1.16

replace k8s.io/client-go => k8s.io/client-go v0.18.0

require (
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/rancher/lasso v0.0.0-20200905045615-7fcb07d6a20b
	github.com/rancher/wrangler v0.7.3-0.20201020003736-e86bc912dfac
	github.com/rancher/wrangler-api v0.6.1-0.20200427172631-a7c2f09b783e
	github.com/sirupsen/logrus v1.6.0
	golang.org/x/crypto v0.0.0-20210513164829-c07d793c2f9a // indirect
	golang.org/x/oauth2 v0.0.0-20201208152858-08078c50e5b5
	google.golang.org/api v0.40.0
	k8s.io/api v0.18.8
	k8s.io/apimachinery v0.18.8
	k8s.io/client-go v12.0.0+incompatible
)
