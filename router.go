package mage

import (
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type Route struct {
	Name string
	Children map[string]Route
	factory func() Controller
}

func newRoute(name string, factory func() Controller) Route {
	children := make(map[string]Route)
	return Route{Name:name, Children:children, factory:factory}
}

const KeyRouteNotFound string = "__mage_not_found"
var ErrRouteNotFound = errors.New("Can't find route")

type Router struct {
	//Optional function to control routing with custom algorithms
	ControllerForPath func(ctx context.Context, path string) (error, Controller)
	routes map[string]Route
}

func NewRouter() Router {
	routes := make(map[string]Route)
	router := Router{routes:routes}
	//add default routes
	router.SetRoute(KeyRouteNotFound, func() Controller {return &NotFoundController{}})
	return router
}

func (router *Router) SetRoute(path string, factory func() Controller) {
	r := newRoute(path, factory)
	router.routes[path] = r
}

func (router Router) controllerForPath(ctx context.Context, path string) (error, Controller) {
	if router.ControllerForPath != nil {
		return router.ControllerForPath(ctx, path)
	}

	err, route := router.searchRoute(path)
	if err == ErrRouteNotFound {
		r := router.routes[KeyRouteNotFound]
		return nil, r.factory()
	}

	if err != nil {
		return err, nil
	}

	controller := route.factory()
	return nil, controller
}

func (router Router) searchRoute(path string) (error, *Route) {
	if route, ok := router.routes[path]; ok {
		return nil, &route
	}
	return ErrRouteNotFound, nil
}


