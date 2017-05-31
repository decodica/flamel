 package mage

import (
	"golang.org/x/net/context"
)

type RequestInputs map[string]requestInput;


type magePage struct {
	page        Page
	out         RequestOutput;
}

type Page interface {
	//the page logic is executed here
	//if the user is valid it is recoverable from the context
	//else the user is nil
	//Process method consumes the context -> context variations, i.e. appengine.Namespace
	//can be used INSIDE the Process function
	Process(ctx context.Context, out *RequestOutput) Redirect;
	//called to release resources
	OnDestroy(ctx context.Context);
}

func newGaemsPage(page Page) *magePage {
	p := &magePage{page:page};
	return p;
}

func (page *magePage) process(ctx context.Context) Redirect {
	//create and package the request
	page.out = newRequestOutput();
	return page.page.Process(ctx, &page.out);
}


func InputsFromContext(ctx context.Context) RequestInputs {
	inputs := ctx.Value(REQUEST_INPUTS).(RequestInputs);
	return inputs;
}