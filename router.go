package mage

import (
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"log"
	"regexp"
	"sort"
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

const paramRegex = `:(\w+)`
const wildcardChar = '*'

var paramTester = regexp.MustCompile(paramRegex)

func newRoute(urlPart string, factory func() Controller) route {
	children := make(map[string]route)
	//analyze the name to determine the route type
	route := route{children:children, factory:factory}

	//check special routes
	if key := extractSpecialKey(urlPart);key != "" {
		route.name = key
		route.routeType = special
		return route
	}

	if par := extractParameter(urlPart); par != "" {
		route.name = urlPart
		route.routeType = parameter
		return route
	}

	if strings.Index(urlPart, string(wildcardChar)) != -1 {
		route.name = urlPart
		route.routeType = wildcard
		return route
	}

	route.name = urlPart
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
	noroot := url[:strings.LastIndex(url, "/")]
	return strings.Split(noroot, "/")
}

//Route class
type route struct {
	name string
	children map[string]route
	factory func() Controller
	routeType routeType
}

func (route route) match(url string) bool {
	//log.Printf("url %s, rest %s, route.Name %s ", url, rest, route.Name)
	switch route.routeType {
	case parameter:
		return true
	case static:
		return route.name == url
	case wildcard:
		return true
	}


	return false
}

//Router class
type Router struct {
	//Optional function to control routing with custom algorithms
	ControllerForPath func(ctx context.Context, path string) (error, Controller)
	root route
	tree *tree
}

var notFoundFactory = func() Controller {return &NotFoundController{}}

func NewRouter() Router {
	router := Router{}
	//add default routes

	root := newRoute("", notFoundFactory)
	router.root = root

	router.tree = newTree()
	return router
}

/*
we add /parent/child/{param}
we want the router to have one route "parent" and that route mush have a child route "child"
 */
/* func (router *Router) SetRoute(path string, factory func() Controller) {

	route := router.root
	parts := parts(path)

	//add subroutes if not exist, and associate the Controller factory with the final segment
	for _, v := range parts {
		if _, ok := route.children[v]; !ok {
			//if we do not have the transitioning node we add it with a nil controller
			route.children[v] = newRoute(v, nil)
		}
		route = route.children[v]
	}

	endpoint := path[strings.LastIndex(path,"/") + 1:]
	route.children[endpoint] = newRoute(endpoint, factory)
}*/

func (router *Router) SetRoute(path string, factory func() Controller) {
	route := newRoute(path, factory)
	router.tree.insert(&route)
}

func (router Router) controllerForPath(ctx context.Context, path string) (error, Controller) {
	if router.ControllerForPath != nil {
		return router.ControllerForPath(ctx, path)
	}

	route := router.tree.getRoute(path)

	if route == nil {
		return errors.New("Route not found"), nil
	}

	controller := route.factory()
	return nil, controller
}

//Routes are being searched using a Greedy Search algorithm, based on the work at https://github.com/blackshadev/Roadie/blob/ts/src/routing/static/route_search.ts
/*func (router Router) searchRoute(path string) (error, *route) {
	//split the url and create the nodes array
	parts := strings.Split(path, "/")
	var nodes []routingState

	//set state for the root route
	initial := newRoutingState(router.root)
	initial.left = parts
	nodes = append(nodes, initial)

	for len(nodes) > 0 {

		state := nodes[0]
		nodes = nodes[1:]

		if len(state.left) == 0 {
			log.Printf(">>> State.left == 0. Returning data: %+v", state.data)
			return nil, &state.data
		}

		next := state.left[0]
		state.left = state.left[1:]

		rest := next
		//if we are
		if len(state.left) > 0 {
			rest = fmt.Sprintf("%s/%s", next, strings.Join(state.left, "/"))
		}

		possibilities := state.getPossibleRoutes(next, rest)
		states := make([]routingState, 0)
		//update each possibility cost
		for _, v := range possibilities {

			ns := state.clone()
			ns.data = v
			ns.path = append(ns.path, v.name)

			switch v.routeType {
			case parameter:
				ns.penalty += 1
				ns.params[v.name] = next
			case wildcard:
				ns.uri = rest
				if ns.uri != "" {
					ns.penalty += len(ns.uri) - len(v.name) - 1
				}
				ns.left = nil
			}
			states = append(states, ns)
		}

		nodes = append(nodes, states...)
	}

	return ErrRouteNotFound, nil
}*/

type routingState struct {
	penalty int
	params map[string]interface{}
	//path leading to this state
	path []string
	//paths left to analyze
	left []string
	//route associated with the given state
	data route

	//collects wildcard leftovers (?)
	uri string
}

func newRoutingState(start route) routingState {
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
	ns.left = state.left
	ns.path = state.path
	ns.penalty = state.penalty
	ns.uri = state.uri
	ns.params = state.params
	ns.data = state.data
	return ns
}

/*func (state routingState) getPossibleRoutes(urlPart string, rest string) []route {
	routes := make([]route, 0)
	for _, child := range state.data.children {
		if child.match(urlPart, rest) {
			routes = append(routes, child)
		}
	}
	return routes
}*/

// Router uses a specialized radix tree implementation to handle matching.
// heavily inspired by @https://github.com/armon/go-radix/blob/master/radix.go
// a route is our leaf node, where route name is the key.

type edge struct {
	label byte
	node *node
}

type edges []edge

func (e edges) Len() int {
	return len(e)
}

// edges implements sortable interface
func (e edges) Less(i, j int) bool {
	if e[i].label == ':' {
		return true
	}

	if e[i].label == '*' && e[j].label != ':' {
		return true
	}

	return e[i].label < e[j].label
}

func (e edges) Swap(i, j int) {
	e[i], e[j] = e[j], e[i]
}

func (e edges) Sort() {
	sort.Sort(e)
}

type node struct {
	route *route

	//prefix is the common prefix to ignore
	prefix string

	// sorted slice of edge
	edges edges
}

func (n *node) isLeaf() bool {
	return n.route != nil
}

func (n *node) addEdge(edge edge) {
	n.edges = append(n.edges, edge)
	n.edges.Sort()
}

func (n *node) updateEdge(label byte, node *node) {
	count := len(n.edges)
	idx := sort.Search(count, func(i int) bool {
		return n.edges[i].label >= label
	})

	// todo: remove control after debugging
	if idx < count && n.edges[idx].label == label {
		n.edges[idx].node = node
		return
	}

	panic("Update on missing edge")
}

func (n *node) getEdge(label byte) *node {
	count := len(n.edges)

	idx := sort.Search(count, func(i int) bool {
		return n.edges[i].label >= label
	})
	if idx < count && n.edges[idx].label == label {
		return n.edges[idx].node
	}

	return nil
}

// retrieves the edge looking for params if a static match is not found
func (n *node) getParametricEdge(label byte) *node {
	res := n.getEdge(label)
	if res == nil {
		if l := len(n.edges); l > 0 {
			if n.edges[0].label == ':' {
				log.Printf("Param at node %+v", n.edges[0].node)
				return n.edges[0].node
			}

			if n.edges[0].label == wildcardChar {
				log.Printf("Wildcard at node %+v", n.edges[0].node)
				return n.edges[0].node
			}
		}

		log.Printf("can't find node for label %s.", string(label))
		// we couldn't find a node. Check if
		for _, v := range n.edges {
			log.Printf("edge.label %s -> node %s", string(v.label), v.node.prefix)
		}
	}

	return res
}

func (n *node) delEdge(label byte) {
	num := len(n.edges)
	idx := sort.Search(num, func(i int) bool {
		return n.edges[i].label >= label
	})
	if idx < num && n.edges[idx].label == label {
		copy(n.edges[idx:], n.edges[idx+1:])
		n.edges[len(n.edges)-1] = edge{}
		n.edges = n.edges[:len(n.edges)-1]
	}
}

type tree struct {
	root *node
	size int
}

func newTree() *tree {
	return &tree{root: &node{}}
}

func longestPrefix(s1 string, s2 string) int {
	min := len(s1)
	if l := len(s2); l < min {
		min = l
	}

	i := 0
	for i < min {
		if s1[i] != s2[i] {
			break
		}
		i++
	}
	return i
}

// adds a new node or updates an existing one
// returns true if the node has been updated
func (t *tree) insert(route *route) (updated bool) {

	var parent *node
	n := t.root
	search := route.name

	for {
		if len(search) == 0 {
			// we append the route at the end of the tree.
			n.route = route

			// if we are not at the leaf, we increment the tree size
			isLeaf := n.isLeaf()
			if !isLeaf {
				t.size++
			}

			return isLeaf
		}

		// look for the edge
		parent = n
		n = n.getEdge(search[0])

		// there is no edge from the parent to the new node.
		// we create a new edge and a new node, using the search as prefix
		// and we connect it to the new node (parent)-----(new-node)
		if n == nil {
			e := edge{
				label: search[0],
				node: &node {
					route: route,
					prefix: search,
				},
			}
			parent.addEdge(e)
			t.size++
			log.Printf("Added node %+v with prefix %v", e.node, search)
			return false
		}

		// we found an edge to attach the new node
		common := longestPrefix(search, n.prefix)

		// if the prefixes coincide in len
		// we empty the search and continue the loop
		// thus adding a node as a leaf or replacing an existing node
		if common == len(n.prefix) {
			search = search[common:]
			continue
		}

		// else, we must add the node by splitting the old node
		t.size++

		// we create an empty new node
		// starting from the end of the common edge
		child := &node{
			prefix: search[:common],
		}
		parent.updateEdge(search[0], child)

		//we add the new edge
		child.addEdge(edge{
			label: n.prefix[common],
			node: n,
		})
		n.prefix = n.prefix[common:]

		// get the remaining slices
		search = search[common:]
		// If the new key is a subset of the parent
		// we add the node to this
		if len(search) == 0 {
			child.route = route
			return false
		}

		// else we create a new edge and we append it
		child.addEdge(edge{
			label: search[0],
			node: &node{
				route: route,
				prefix: search,
			},
		})

		return false
	}
}

func (t *tree) getRoute(s string) *route {
	n := t.root
	search := s
	log.Printf("Root has edges: %+v", n.edges)
	for _, v := range n.edges {
		log.Printf("Edge %d has node: %+v with prefix %s", v.label, v.node, v.node.prefix)
	}

	for {

		// we traversed all the trie
		// return the route at the node
		if len(search) == 0 {
			log.Printf("Search len is 0. Returning route %+v", n.route)
			return n.route
		}

		n = n.getParametricEdge(search[0])
		log.Printf("Node %+v", n)
		if n == nil {
			log.Print("Node == nil")
			return nil
		}

		if !strings.HasPrefix(search, n.prefix) {
			log.Printf("HasPrefix is false: search: %s, prefix: %s", search, n.prefix)

			// handle special patterns
			if n.route.match(n.prefix) {
				idx := strings.Index(search, "/")
				//todo: parse params
				// the wildcard character was at a leaf
				// we are done with searching
				if idx == - 1 {
					search = ""
				} else {
					search = search[idx:]
				}

				continue
			}

			break
		}

		search = search[len(n.prefix):]
		log.Printf("Search is %s", search)
	}

	return nil
}




