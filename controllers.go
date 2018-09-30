package mage

import (
	"golang.org/x/net/context"
	"net/http"
)

type NotFoundController struct{}

func (c *NotFoundController) Process(ctx context.Context, out *ResponseOutput) Redirect {
	return Redirect{Status: http.StatusNotFound}
}

func (c *NotFoundController) OnDestroy(ctx context.Context) {}
