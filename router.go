package mage

import (
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"strings"
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

func (route Route) match(url string) bool {
	//todo: match name of the endpoint
	if route.Name == url {
		return true
	}

	//todo: match the regexp

	return false
}

func parts(url string) []string {
	parents := url[:strings.LastIndex(url, "/")]
	return strings.Split(parents, "/")
}

const KeyRouteNotFound string = "__mage_not_found"
var ErrRouteNotFound = errors.New("Can't find route")

type Router struct {
	//Optional function to control routing with custom algorithms
	ControllerForPath func(ctx context.Context, path string) (error, Controller)
	routes map[string]Route
}

var notFoundFactory = func() Controller {return &NotFoundController{}}

func NewRouter() Router {
	routes := make(map[string]Route)
	router := Router{routes:routes}
	//add default routes
	router.SetRoute(KeyRouteNotFound, notFoundFactory)
	return router
}

/*
we add /parent/child/{param}
we want the router to have one route "parent" and that route mush have a child route "child"
 */
func (router *Router) SetRoute(path string, factory func() Controller) {
	if path == KeyRouteNotFound {
		router.routes[KeyRouteNotFound] = newRoute(KeyRouteNotFound, notFoundFactory)
		return
	}

	routes := router.routes
	parts := parts(path)

	//add subroutes if not exist, and associate the Controller factory with the final segment
	for _, v := range parts {
		if _, ok := routes[v]; !ok {
			//if we do not have the transitioning node we add it
			routes[v] = newRoute(v, nil)
		}
		routes = routes[v].Children
	}

	endpoint := path[strings.LastIndex(path,"/") + 1:]
	routes[endpoint] = newRoute(endpoint, factory)
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


