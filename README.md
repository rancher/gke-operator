# gke-operator

GKE operator is a Kubernetes CRD controller that controls cluster provisioning in Google Kubernetes Engine using an GKEClusterConfig defined by a Custom Resource Definition.

## Build

```sh
go build -o gke-operator main.go
```

## Test

With `KUBECONFIG` set in your shell, run the binary

```sh
./gke-operator
```

Apply the CRD

```sh
kubectl apply -f crds/gkeclusterconfig.yaml
```

Create a file named `googlecredentialConfig-authEncodedJson` with the contents
of your JSON service account credential. Then create a cloud credential secret:

```sh
kubectl --namespace cattle-global-data create secret generic --from-file=googlecredentialConfig-authEncodedJson cc-abcde
```

Edit at a minimum the `projectID` and create a cluster

```sh
kubectl apply -f examples/cluster-basic.yaml
```

## Develop

The easiest way to debug and develop the GKE operator is to replace the default operator on a running Rancher instance with your local one.

* Run a local Rancher server
* Provision a GKE cluster
* Scale the gke-operator deployment to replicas=0 in the Rancher UI
* Open the gke-operator repo in Goland, set `KUBECONFIG=<kubeconfig_path>` in Run Configuration Environment
* Run the gke-operator in Debug Mode
* Set breakpoints

## Release

#### When should I release?

A KEv2 operator should be released if

* There have been several commits since the last release,
* You need to pull in an update/bug fix/backend code to unblock UI for a feature enhancement in Rancher
* The operator needs to be unRC for a Rancher release

#### How do I release?

Tag the latest commit on the `master` branch. For example, if latest tag is `v1.1.5` you would tag `v1.1.6-rc1`.

    git pull upstream master --tags     // get the latest upstream changes (not your fork)
    git tag v1.1.6-rc1                  // tag HEAD
    git push upstream v1.1.6-rc1        // push the tag

Submit a [rancher/charts PR](https://github.com/rancher/charts/pull/2242) to update the operator and operator-crd chart versions.
Submit a [rancher/rancher PR](https://github.com/rancher/rancher/pull/39745) to update the bundled chart.

#### How do I unRC?

UnRC is the process of removing the rc from a KEv2 operator tag and means the released version is stable and ready for use. Release the KEv2 operator but instead of bumping the rc, remove the rc. For example, if the latest release of GKE operator is `v1.1.5`, that has no rc so release that version or `v1.1.6` if updates are available.
