package mage

import (
	"fmt"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"log"
	"regexp"
	"strings"
)

type routeType int

const (
	static = iota
	parameter
	wildcard
	special
)

//Key to identify the not found route. Used to set the custom notfound controller
const KeyRouteNotFound string = "__mage_not_found"
var ErrRouteNotFound = errors.New("Can't find route")

const paramRegex = `{(\w+)}`
const wildcardChar = "*"

var paramTester = regexp.MustCompile(paramRegex)

func newRoute(urlPart string, factory func() Controller) Route {
	children := make(map[string]Route)
	//analyze the name to determine the route type
	route := Route{Children:children, factory:factory}

	//check special routes
	if key := extractSpecialKey(urlPart);key != "" {
		route.Name = key
		route.routeType = special
		return route
	}

	if par := extractParameter(urlPart);par != "" {
		route.Name = par
		route.routeType = parameter
		return route
	}

	if strings.Index(urlPart, wildcardChar) != -1 {
		route.Name = urlPart
		route.routeType = wildcard
		return route
	}

	route.Name = urlPart
	route.routeType =static
	return route
}

func extractParameter(par string) string {
	return paramTester.FindString(par)
}

func extractSpecialKey(url string) string {
	if url == KeyRouteNotFound {
		return KeyRouteNotFound
	}
	return ""
}

func parts(url string) []string {
	parents := url[:strings.LastIndex(url, "/")]
	return strings.Split(parents, "/")
}

//Route class
type Route struct {
	Name string
	Children map[string]Route
	factory func() Controller
	routeType routeType
}

func (route Route) match(url string, rest string) bool {
	log.Printf("url %s, rest %s, route.Name %s ", url, rest, route.Name)
	switch route.routeType {
	case parameter:
		return true
	case static:
		return route.Name == url
	case wildcard:
		//todo: test wildcard
		return route.Name == url
	}

	return false
}

//Router class
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
	//split the url and create the nodes array
	parts := strings.Split(path, "/")
	nodes := make([]routingState, len(parts))

	//set state for the root route
	initial := newRoutingState(router.routes[""])
	initial.left = parts
	nodes[0] = initial

	for len(nodes) > 0 {
		//grab the first node and check if it's a goal state
		state, nodes := nodes[0], nodes[1:]
		if len(state.left) == 0 {
			return nil, &state.data
		}

		//get next url to check
		next := state.left[0]
		state.left = state.left[1:]

		rest := next
		//if we are
		if len(state.left) > 0 {
			rest = fmt.Sprintf("%s/%s", next, strings.Join(state.left, "/"))
		}

		possibilities := state.getPossibleRoutes(next, rest)
		states := make([]routingState, 0)
		for _, v := range possibilities {

			ns := state.clone()
			ns.data = v
			ns.path = append(ns.path, v.Name)

			switch v.routeType {
			case parameter:
				ns.penalty += 1
				ns.params[v.Name] = next
			case wildcard:
				ns.uri = rest
				if rest != "" {
					ns.penalty += len(ns.uri) - len(v.Name) - 1
				}
				ns.left = nil
			}
			states = append(states, ns)
		}
		nodes = append(nodes, states...)
	}

	if route, ok := router.routes[path]; ok {
		return nil, &route
	}
	return ErrRouteNotFound, nil
}

//Routes are being searched using a Greedy Search algorithm, based on the work at https://github.com/blackshadev/Roadie/blob/ts/src/routing/static/route_search.ts
type routingState struct {
	penalty int
	params map[string]interface{}
	//path leading to this state
	path []string
	//paths left to analyze
	left []string

	data Route

	//collects wildcard leftovers (?)
	uri string
}

func newRoutingState(start Route) routingState {
	s := routingState{data: start}
	s.params = make(map[string]interface{})
	s.path = make([]string, 0)
	s.left = make([]string, 0)
	return s
}

//the greedy search cost function
func (state routingState) cost() int {
	return len(state.path) + state.penalty
}

func (state routingState) clone() routingState {
	ns := routingState{}
	ns.left = state.left[1:]
	ns.path = state.path[1:]
	ns.penalty = state.penalty
	ns.uri = state.uri
	ns.params = state.params
	ns.data = state.data
	return ns
}

func (state routingState) getPossibleRoutes(urlPart string, rest string) []Route {
	routes := make([]Route, 0)
	for _, child := range state.data.Children {
		if child.match(urlPart, rest) {
			routes = append(routes, child)
		}
	}
	return routes
}