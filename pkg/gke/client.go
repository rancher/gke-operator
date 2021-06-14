package gke

import (
	"context"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gkeapi "google.golang.org/api/container/v1"
	"google.golang.org/api/option"
)

// GetGKEClient accepts a JSON credential string and returns a Service client.
func GetGKEClient(ctx context.Context, credential string) (*gkeapi.Service, error) {
	ts, err := GetTokenSource(ctx, credential)
	if err != nil {
		return nil, err
	}
	return getServiceClientWithTokenSource(ctx, ts)
}

func getServiceClientWithTokenSource(ctx context.Context, ts oauth2.TokenSource) (*gkeapi.Service, error) {
	return gkeapi.NewService(ctx, option.WithHTTPClient(oauth2.NewClient(ctx, ts)))
}

func GetTokenSource(ctx context.Context, credential string) (oauth2.TokenSource, error) {
	ts, err := google.CredentialsFromJSON(ctx, []byte(credential), gkeapi.CloudPlatformScope)
	if err != nil {
		return nil, err
	}
	return ts.TokenSource, nil
}
