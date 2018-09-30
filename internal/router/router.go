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

const RoutingParamsKey = "__routing_params"

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

//
type Param struct {
	Key   string
	Value string
}

type Params []Param

//Route class
type Route struct {
	Name      string
	Handler func(ctx context.Context) interface{}
	// factory   func() Controller
	routeType routeType
}

func NewRoute(url string, handler func(ctx context.Context) interface{}) Route {
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

//Router class
type Router struct {
	tree              *tree
}

func NewRouter() Router {
	router := Router{}
	router.tree = newTree()
	return router
}

func (router *Router) SetRoute(path string, handler func(ctx context.Context) interface{}) {
	route := NewRoute(path, handler)
	router.tree.insert(&route)
}

func (router *Router) RouteForPath(ctx context.Context, path string) (error, interface{}) {
	route, params := router.tree.findRoute(path)

	if route == nil {
		return ErrRouteNotFound, nil
	}

	c := context.WithValue(ctx, RoutingParamsKey, params)
	controller := route.Handler(c)
	return nil, controller
}
