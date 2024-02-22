package main

import (
	"fmt"
	"os"

	gkev1 "github.com/rancher/gke-operator/pkg/apis/gke.cattle.io/v1"
	_ "github.com/rancher/wrangler-api/pkg/generated/controllers/apiextensions.k8s.io"
	controllergen "github.com/rancher/wrangler/v2/pkg/controller-gen"
	"github.com/rancher/wrangler/v2/pkg/controller-gen/args"
	"github.com/rancher/wrangler/v2/pkg/crd"
	"github.com/rancher/wrangler/v2/pkg/yaml"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func main() {
	os.Unsetenv("GOPATH")

	controllergen.Run(args.Options{
		OutputPackage: "github.com/rancher/gke-operator/pkg/generated",
		Boilerplate:   "pkg/codegen/boilerplate.go.txt",
		Groups: map[string]args.Group{
			"gke.cattle.io": {
				Types: []interface{}{
					"./pkg/apis/gke.cattle.io/v1",
				},
				GenerateTypes: true,
			},
			corev1.GroupName: {
				Types: []interface{}{
					corev1.Pod{},
					corev1.Node{},
					corev1.Secret{},
				},
			},
		},
	})

	gkeClusterConfig := newCRD(&gkev1.GKEClusterConfig{}, func(c crd.CRD) crd.CRD {
		c.ShortNames = []string{"gkecc"}
		return c
	})

	obj, err := gkeClusterConfig.ToCustomResourceDefinition()
	if err != nil {
		panic(err)
	}

	obj.(*unstructured.Unstructured).SetAnnotations(map[string]string{
		"helm.sh/resource-policy": "keep",
	})

	gkeCCYaml, err := yaml.Export(obj)
	if err != nil {
		panic(err)
	}

	if err := saveCRDYaml("gke-operator-crd", string(gkeCCYaml)); err != nil {
		panic(err)
	}

	fmt.Printf("obj yaml: %s", gkeCCYaml)
}

func newCRD(obj interface{}, customize func(crd.CRD) crd.CRD) crd.CRD {
	crd := crd.CRD{
		GVK: schema.GroupVersionKind{
			Group:   "gke.cattle.io",
			Version: "v1",
		},
		Status:       true,
		SchemaObject: obj,
	}
	if customize != nil {
		crd = customize(crd)
	}
	return crd
}

func saveCRDYaml(name, yaml string) error {
	filename := fmt.Sprintf("./charts/%s/templates/crds.yaml", name)
	save, err := os.Create(filename)
	if err != nil {
		return err
	}

	defer save.Close()
	if err := save.Chmod(0755); err != nil {
		return err
	}

	if _, err := fmt.Fprint(save, yaml); err != nil {
		return err
	}

	return nil
}
