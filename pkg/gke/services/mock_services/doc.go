package mock_services

// Run go generate to regenerate this mock.
//
//go:generate go tool -modfile ../../../../gotools/mockgen/go.mod mockgen -destination gke_mock.go -package mock_services -source ../gke.go GKEClusterServiceInerface,GKENodePoolServiceInerface
