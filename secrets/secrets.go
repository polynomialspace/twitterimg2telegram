package secrets

import (
	"context"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
)

func Get(projectID string, secret string) ([]byte, error) {
	ctx := context.Background()
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, err
	}

	baseName := "projects/" + projectID + "/secrets/"
	versions := `/versions/latest`
	accessRequest := &secretmanagerpb.AccessSecretVersionRequest{
		Name: baseName + secret + versions,
	}
	response, err := client.AccessSecretVersion(ctx, accessRequest)
	return response.Payload.GetData(), err
}
