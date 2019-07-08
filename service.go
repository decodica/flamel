package flamel

import "context"

type Service interface {
	Name() string
	// used to set the service up
	Initialize()
	// called everytime a request is being processed
	OnStart(ctx context.Context) context.Context
	// called once the request has been processed
	OnEnd(ctx context.Context)
}
