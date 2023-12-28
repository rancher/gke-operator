package controller

import (
	"context"
	"fmt"
	"strings"

	gkev1 "github.com/rancher/gke-operator/pkg/apis/gke.cattle.io/v1"
	"github.com/rancher/gke-operator/pkg/gke"
	wranglerv1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"golang.org/x/oauth2"
	gkeapi "google.golang.org/api/container/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func parseCredential(ref string) (namespace string, name string) {
	parts := strings.SplitN(ref, ":", 2)
	if len(parts) == 1 {
		return "", parts[0]
	}
	return parts[0], parts[1]
}

func GetSecret(_ context.Context, secretsClient wranglerv1.SecretClient, configSpec *gkev1.GKEClusterConfigSpec) (string, error) {
	ns, id := parseCredential(configSpec.GoogleCredentialSecret)
	secret, err := secretsClient.Get(ns, id, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	dataBytes, ok := secret.Data["googlecredentialConfig-authEncodedJson"]
	if !ok {
		return "", fmt.Errorf("could not read malformed cloud credential secret %s from namespace %s", id, ns)
	}
	return string(dataBytes), nil
}

func GetCluster(ctx context.Context, secretsClient wranglerv1.SecretClient, configSpec *gkev1.GKEClusterConfigSpec) (*gkeapi.Cluster, error) {
	cred, err := GetSecret(ctx, secretsClient, configSpec)
	if err != nil {
		return nil, err
	}
	gkeClient, err := gke.GetGKEClusterClient(ctx, cred)
	if err != nil {
		return nil, err
	}
	return gke.GetCluster(ctx, gkeClient, configSpec)
}

func GetTokenSource(ctx context.Context, secretsClient wranglerv1.SecretClient, configSpec *gkev1.GKEClusterConfigSpec) (oauth2.TokenSource, error) {
	cred, err := GetSecret(ctx, secretsClient, configSpec)
	if err != nil {
		return nil, fmt.Errorf("error getting secret: %w", err)
	}
	ts, err := gke.GetTokenSource(ctx, cred)
	if err != nil {
		return nil, fmt.Errorf("error getting oauth2 token: %w", err)
	}
	return ts, nil
}

// BuildUpstreamClusterState creates an GKEClusterConfigSpec (spec for the GKE cluster state) from the existing
// cluster configuration.
func BuildUpstreamClusterState(ctx context.Context, secretsCache wranglerv1.SecretCache, secretClient wranglerv1.SecretClient, configSpec *gkev1.GKEClusterConfigSpec) (*gkev1.GKEClusterConfigSpec, error) {
	cred, err := GetSecret(ctx, secretClient, configSpec)
	if err != nil {
		return nil, err
	}
	gkeClient, err := gke.GetGKEClusterClient(ctx, cred)
	if err != nil {
		return nil, err
	}
	gkeCluster, err := gke.GetCluster(ctx, gkeClient, configSpec)
	if err != nil {
		return nil, err
	}

	h := Handler{
		secretsCache: secretsCache,
		secrets:      secretClient,
	}
	return h.buildUpstreamClusterState(gkeCluster)
}
