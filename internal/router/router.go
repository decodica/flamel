package router

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

type routeType int

const (
	static = iota
	parameter
	wildcard
)

var ErrRouteNotFound = errors.New("can't find route")

const RoutingParamsKey = "__flamel_routing_params__"

const paramRegex = `:(\w+)`
const wildcardChar = '*'
const paramChar = ':'

var paramTester = regexp.MustCompile(paramRegex)

func extractParameter(par string) string {
	if !paramTester.MatchString(par) {
		return ""
	}
	return paramTester.FindStringSubmatch(par)[1]
}

type Param struct {
	Key   string
	Value string
}

type Params []Param

//Route class
type Route struct {
	Name    string
	Handler func(ctx context.Context) (interface{}, context.Context)
	// factory   func() Controller
	routeType routeType
}

func NewRoute(url string, handler func(ctx context.Context) (interface{}, context.Context)) Route {
	//analyze the name to determine the route type
	route := Route{Handler: handler}

	if par := extractParameter(url); par != "" {
		route.Name = url
		route.routeType = parameter
		return route
	}

	if strings.Index(url, string(wildcardChar)) != -1 {
		route.Name = url
		route.routeType = wildcard
		return route
	}

	route.Name = url
	route.routeType = static
	return route
}

func (route Route) match(url string) bool {
	//log.Printf("url %s, rest %s, route.Name %s ", url, rest, route.Name)
	switch route.routeType {
	case parameter:
		return true
	case static:
		return route.Name == url
	case wildcard:
		return true
	}

	return false
}

type Router struct {
	tree *tree
}

func NewRouter() Router {
	router := Router{}
	router.tree = newTree()
	return router
}

// Creates the path - route relationship.
// handler is invoked once the route is found
func (router *Router) SetRoute(path string, handler func(ctx context.Context) (interface{}, context.Context)) {
	route := NewRoute(path, handler)
	router.tree.insert(&route)
}

// Given the path it returns the assigned route from the radix tree
func (router *Router) RouteForPath(ctx context.Context, path string) (context.Context, error, interface{}) {
	route, params := router.tree.findRoute(path)

	if route == nil {
		return ctx, ErrRouteNotFound, nil
	}

	c := context.WithValue(ctx, RoutingParamsKey, params)
	controller, c := route.Handler(c)
	return c, nil, controller
}
