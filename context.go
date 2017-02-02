package mage

import (
	"golang.org/x/net/context"
	"github.com/pkg/errors"
)

type Context struct {
	context.Context
	meta map[requestItem]requestInput
}

func (ctx *Context) Meta(req requestItem) (requestInput, error) {
	val, ok := ctx.meta[req];

	if !ok {
		return requestInput{}, errors.New("Requested meta is not set by the request");
	}

	return val, nil;
}