package mage

import (
	"context"
	"github.com/pkg/errors"
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
)

var ErrRouteNotFound = errors.New("Can't find route")

const paramRegex = `:(\w+)`
const wildcardChar = '*'
const paramChar = ':'

const segmentRegex = `^/?(\w+)`

var paramTester = regexp.MustCompile(paramRegex)
var segmentTester = regexp.MustCompile(segmentRegex)

func newRoute(urlPart string, factory func() Controller) route {
	//analyze the name to determine the route type
	route := route{factory:factory}

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
	route.routeType = static
	return route
}

func extractParameter(par string) string {
	if !paramTester.MatchString(par) {
		return ""
	}
	return paramTester.FindStringSubmatch(par)[1]
}

func paramMatch(par string) string {
	return paramTester.FindString(par)
}

type param struct {
	key string
	value string
}

type params []param

func (p *params) add(key string, value string) {
	param := param{
		key: key,
		value: value,
	}
	*p = append(*p, param)
}

//Route class
type route struct {
	name string
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
	ControllerForPath func(ctx context.Context, path string) (error, Controller, params)
	tree *tree
}

func NewRouter() Router {
	router := Router{}
	router.tree = newTree()
	return router
}

func (router *Router) SetRoute(path string, factory func() Controller) {
	route := newRoute(path, factory)
	router.tree.insert(&route)
}

func (router Router) controllerForPath(ctx context.Context, path string) (error, Controller, params) {
	if router.ControllerForPath != nil {
		return router.ControllerForPath(ctx, path)
	}

	route, params := router.tree.getRoute(path)

	if route == nil {
		return ErrRouteNotFound, nil, nil
	}

	controller := route.factory()
	return nil, controller, params
}

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

	// number of wildcard as direct children
	wildcardCount int

	// number of parameters as direct children
	parameterCount int

	//prefix is the common prefix to ignore
	prefix string

	// sorted slice of edge
	edges edges
}

func (n node) isParametrized() bool {
	return n.parameterCount + n.wildcardCount > 0
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

	if idx < count && n.edges[idx].label == label {
		n.edges[idx].node = node
		log.Printf("Node %s is now at edge %s of parent %s", node.prefix, string(n.edges[idx].label), n.prefix)
		return
	}

	panic("Update on missing edge")
}

func (n *node) getEdge(label byte) *node {
	log.Printf("Reading node %s, looking for %s", n.prefix, string(label))
	for i, v := range n.edges {
		log.Printf("edge.label[%d] %s -> node %s", i, string(v.label), v.node.prefix)
	}

	count := len(n.edges)

	idx := sort.Search(count, func(i int) bool {
		return n.edges[i].label >= label
	})
	if idx < count && n.edges[idx].label == label {
		return n.edges[idx].node
	}

	return nil
}

func (n *node) getParametricEdge() *node {
	if n.wildcardCount > n.parameterCount {
		return n.getEdge(wildcardChar)
	}
	return n.getEdge(paramChar)
}

type tree struct {
	root *node
	size int
}

func newTree() *tree {
	return &tree{root: &node{}}
}

func longestPrefix(k1, k2 string) int {
	max := len(k1)
	if l := len(k2); l < max {
		max = l
	}
	var i int
	for i = 0; i < max; i++ {
		if k1[i] != k2[i] {
			break
		}
	}
	log.Printf("Confronting %s and %s. Common slice at %d: %s", k1, k2, i, k1[:i])
	return i
}

func minPrefix(p1, p2 int) int {
	if p1 > p2 {
		return p2
	}
	return p1
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
		// or we have an empty tree
		if n == nil {
			log.Printf("Adding node for search %s under parent %s", search, parent.prefix)
			e := edge{
				label: search[0],
				node: &node {
					route: route,
					prefix: search,
				},
			}
			parent.addEdge(e)

			switch route.routeType {
			case parameter:
				parent.parameterCount++
			case wildcard:
				parent.wildcardCount++
			}

			t.size++
			return false
		}

		// we found an edge to attach the new node
		// common is the idx of the divergent char
		// i.e. "aab" and "aac" then common has value 2
		wanted := longestPrefix(search, n.prefix)
		//todo slash := strings.IndexByte(search, '/')

		// if the prefixes coincide in len
		// we consume the search and continue the loop with the remaining slice.
		// we have this case when ex.confronting /static with /static/enzo. In this case the common chars
		// are equal to the node prefix (/static).
		// We walk the node and look for a place to append the route following this path
		if wanted == len(n.prefix) {
			search = search[wanted:]
			log.Printf("Searching where to place %s", search)
			continue
		}

		// else, we must add the node by splitting the old node
		t.size++

		// We split the current node to account for common parts.
		// the new child has the prefix in common.
		// ex. /static/carlo with /static/enzo -> the common route is /static/
		// thus we create a new route-less node with prefix "/static/"
		// child is the new transition node
		child := &node{
			prefix: search[:wanted],
		}
		parent.updateEdge(search[0], child)

		// now that we splitted the nodes, we re-prepend the newly created node (created from the split)
		// to the common part.
		// ex. we are inserting "/static/enzo" and we find "/static/carlo"
		// in this case we create a new node with prefix "/static/", we append the "carlo" to the "/static" node
		// and we add "enzo" to the static node
		// we must update the state of the wildcardChild, checking if any wildcard are left
		e := edge{
			label: n.prefix[wanted],
			node: n,
		}
		child.addEdge(e)
		n.prefix = n.prefix[wanted:]

		log.Printf("After moving the nodes, we re-added child %s at edge label %s to parent %s", n.prefix, string(e.label), child.prefix)
		if n.route != nil {
			switch n.route.routeType {
			case parameter:
				child.parameterCount++
				parent.parameterCount--
			case wildcard:
				child.wildcardCount++
				parent.wildcardCount--
			}
		}
		// get the remaining slices
		search = search[wanted:]
		// If the new key is a subset of the parent
		// we add the node to this
		if len(search) == 0 {
			child.route = route
			return false
		}

		// else we create a new edge and we append it
		e = edge{
			label: search[0],
			node: &node{
				route: route,
				prefix: search,
			},
		}
		child.addEdge(e)
		switch route.routeType {
		case parameter:
			child.parameterCount++
		case wildcard:
			child.wildcardCount++
		}
		log.Printf("After splitting the nodes, we add child %s at edge label %s to parent %s", e.node.prefix, string(e.label), child.prefix)

		return false
	}
}

// recursiveWalk is used to do a pre-order walk of a node
// recursively. Returns true if the walk should be aborted
func recursiveWalk(n *node, path string) bool {
	if n.route != nil && n.route.name == path {
		name := ""
		r := n.route != nil; if r {
			name = n.route.name
		}
		log.Printf("walking node with prefix %s. It holds route %s", n.prefix, name)
		return true
	}

	for _, e := range n.edges {
		log.Printf("Moving from node %s to %s through edge labeled '%s'. \"%s\" node has %d wildcards and %d parameters", n.prefix, e.node.prefix, string(e.label), n.prefix, n.wildcardCount, n.parameterCount)
		if recursiveWalk(e.node, path) {
			return true
		}
	}

	return false
}

func (t *tree) getRoute(s string) (*route, params) {
	n := t.root
	search := s

	// maps all params gathered along the path
	var params params

	log.Printf("LOOKING FOR ROUTE %s", s)
	for {

		// we traversed all the trie
		// return the route at the node
		if len(search) == 0 {
			return n.route, params
		}

		parent := n
		edge := search[0]
		n = n.getEdge(edge)
		// no edge found, route does not exist
		if n == nil || !strings.HasPrefix(search, n.prefix) {
			log.Printf("No node found at edge %s while searching for %s. Parent.prefix: %s, Widlcards: %d, Parameters %d. Route is %+v",
				string(edge), search, parent.prefix, parent.wildcardCount, parent.parameterCount, n)

			if parent.isParametrized() {
				// we couldn't find a match, so we go back one level
				// and we check if there's a wildcard or a parameter at the parent level.
				// If so, we walk the wildcard route looking for the correct match.

				// check if we are at the end of the search, assuming no backslashes as route end
				idx := strings.IndexByte(search, '/')

				// we are processing the last path segment, time to extract parameters
				if idx == -1 {
					// we found a terminal wildcard, i.e. "\example\enzo" with route "\example\*"
					// return the node route
					n = parent.getParametricEdge()
					if n == nil {
						break
					}
					return n.route, nil
				}

				n = parent
				search = strings.Replace(search, search[:idx], "*", 1)
				log.Printf("Search has been consumed. Searching for: %s", search)
				continue
			}
			break
		}

		search = search[len(n.prefix):]
	}

	return nil, nil
}



