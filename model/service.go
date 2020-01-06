package model

import (
	"cloud.google.com/go/datastore"
	"context"
	"fmt"
	"os"
)

const name = "__flamel_model_service"
const keyDatastoreClient = "__model_ds_client"

type Service struct {
	project string
}

func ClientFromContext(ctx context.Context) *datastore.Client {
	return ctx.Value(keyDatastoreClient).(*datastore.Client)
}

func (service *Service) Name() string {
	return name
}

func (service *Service) Initialize() {
	service.project = os.Getenv("DATASTORE_PROJECT_ID")
}

// adds the appengine client to the context
func (service *Service) OnStart(ctx context.Context) context.Context {
	client, err := datastore.NewClient(ctx, service.project)
	if err != nil {
		panic(fmt.Errorf("error initializing service %s: %s", service.Name(), err.Error()))
	}
	return context.WithValue(ctx, keyDatastoreClient, client)

}

func (service *Service) OnEnd(ctx context.Context) {
	client := ctx.Value(keyDatastoreClient).(*datastore.Client)
	if err := client.Close(); err != nil {
		panic(fmt.Errorf("unable to close datastore client: %s", err.Error()))
	}
}

func (service *Service) Destroy() {}
