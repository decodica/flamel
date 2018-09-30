package mage

import (
	"golang.org/x/net/context"
)

type RequestInputs map[string]requestInput

func InputsFromContext(ctx context.Context) RequestInputs {
	inputs := ctx.Value(KeyRequestInputs).(RequestInputs)
	return inputs
}

type Controller interface {
	//the page logic is executed here
	//Process method consumes the context -> context variations, i.e. appengine.Namespace
	//can be used INSIDE the Process function
	Process(ctx context.Context, out *ResponseOutput) Redirect
	//called to release resources
	OnDestroy(ctx context.Context)
}
