# gke-operator

Operator for managing GKE clusters in Rancher.

## Building

```sh
go build -o gke-operator main.go
```

## Running

With a kubeconfig set in your shell, run the binary:

```sh
./gke-operator
```

Apply the CRD:

```sh
kubectl apply -f crds/gkeclusterconfig.yaml
```

Create a file named `googlecredentialConfig-authEncodedJson` with the contents
of your JSON service account credential. Then create a cloud credential secret:

```sh
kubectl --namespace cattle-global-data create secret generic --from-file=googlecredentialConfig-authEncodedJson cc-abcde
```

Edit at a minimum the projectID and create a cluster:

```sh
kubectl apply -f examples/cluster-basic.yaml
```
