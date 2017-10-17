package mage

import (
	"golang.org/x/net/context"
)

type RequestInputs map[string]requestInput

type innerResponse struct {
	response        Response
	out         ResponseOutput
}

type Response interface {
	//the page logic is executed here
	//if the user is valid it is recoverable from the context
	//else the user is nil
	//Process method consumes the context -> context variations, i.e. appengine.Namespace
	//can be used INSIDE the Process function
	Process(ctx context.Context, out *ResponseOutput) Redirect
	//called to release resources
	OnDestroy(ctx context.Context)
}

func newInnerResponse(res Response) *innerResponse {
	p := &innerResponse{response:res}
	return p
}

func (res *innerResponse) process(ctx context.Context) Redirect {
	//create and package the request
	res.out = newRequestOutput()
	return res.response.Process(ctx, &res.out)
}


func InputsFromContext(ctx context.Context) RequestInputs {
	inputs := ctx.Value(REQUEST_INPUTS).(RequestInputs)
	return inputs
}