package mage

import (
	"context"
	"distudio.com/mage/internal/router"
)

type Router interface {
	SetRoute(url string, handler func(ctx context.Context) Controller)

	RouteForPath(ctx context.Context, path string) (context.Context, error, Controller)
}

type DefaultRouter struct {
	router.Router
}

func NewDefaultRouter() *DefaultRouter {
	dr := DefaultRouter{}
	dr.Router = router.NewRouter()
	return &dr
}

func RoutingParams(ctx context.Context) RequestInputs {
	if params, ok := ctx.Value(router.RoutingParamsKey).(router.Params); ok {
		inputs := make(RequestInputs, len(params))
		for _, p := range params {
			i := requestInput{}
			i.values = []string{p.Value}
			inputs[p.Key] = i
		}
		return inputs
	}
	return nil
}

func (router *DefaultRouter) SetRoute(url string, handler func(ctx context.Context) Controller) {
	router.Router.SetRoute(url, func(ctx context.Context) interface{} {
		return handler(ctx)
	})
}

func (router *DefaultRouter) RouteForPath(ctx context.Context, path string) (context.Context, error, Controller) {
	c, err, controller := router.Router.RouteForPath(ctx, path)
	if err != nil {
		return c, err, nil
	}
	return c, nil, controller.(Controller)
}
