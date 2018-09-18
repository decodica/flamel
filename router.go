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
	route.routeType =static
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
	log.Printf("Reading node %s", n.prefix)
	for i, v := range n.edges {
		log.Printf("edge.label[%d] %s -> node %s", i, string(v.label), v.node.prefix)
	}

	count := len(n.edges)
		/*if n.edges[0].label == ':' {
			log.Printf("Param at node %+v", n.edges[0].node)
			return n.edges[0].node
		}*/
	idx := sort.Search(count, func(i int) bool {
		return n.edges[i].label == ':'
	}); if idx != count {
		return n.edges[idx].node
	}

	res := n.getEdge(label)
	if res != nil {
		log.Printf("NOT WILDCARD. RES IS %s", res.prefix)
		return res
	}

	idx = sort.Search(count, func(i int) bool {
		log.Printf("label %s", string(n.edges[i].label))
		return n.edges[i].label == wildcardChar
	}); if idx != count {
		log.Printf("WILDCARD!")
		return n.edges[idx].node
	}

	return nil
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
			t.size++
			return false
		}

		// we found an edge to attach the new node
		// common is the idx of the divergent char
		// i.e. "aab" and "aac" then common has value 2
		common := longestPrefix(search, n.prefix)

		// if the prefixes coincide in len
		// we empty the search and continue the loop
		// thus adding a node as a leaf or replacing an existing node
		if common == len(n.prefix) {
			search = search[common:]
			log.Printf("Searching where to place %s", search)
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

		e := edge{
			label: search[0],
			node: &node{
				route: route,
				prefix: search,
			},
		}
		child.addEdge(e)

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
		log.Printf("Walking through edge labeled '%s'", string(e.label))
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

	for {

		// we traversed all the trie
		// return the route at the node
		if len(search) == 0 {
			return n.route, params
		}

		edge := search[0]
		n = n.getParametricEdge(edge)
		// no edge found, route does not exist
		if n == nil {
			log.Printf("No node found at edge %s", string(edge))
			return nil, nil
		}

		if !strings.HasPrefix(search, n.prefix) {
			log.Printf("HasPrefix is false: search: %s, prefix: %s", search, n.prefix)

			// if we have a sibling at the edge the nodes share a "/" slashes
			// and the node prefix begins without a slash
			// else the node has no sibling thus the node prefix begins with a slash
			// thus edge == '/' means that the node has at least one sibling (branch)

			// when reading params or regex/wildcards we must determine if the node has children
			// if it does, the algorithm must keep walking the path
			// else we reached a leaf and we can return
			// we must account for ending slashes, such as "parent/child/grandchild/
			// in this case grandchild is a legitimate leaf.
			// Thus a leaf can end with a backslash

			if n.route != nil && n.route.match(n.prefix) {
				// check if the search string has further children.
				// if it does, we keep searching.
				// else we reached a leaf
				matches := segmentTester.FindStringSubmatch(search)
				segment := matches[0]
				param := matches[1]
				log.Printf("Matches: %+v", matches)
				if n.route.routeType == parameter {
					pname := extractParameter(n.prefix)
					params.add(pname, param)
					log.Printf("Added param %s -> %s", pname, param)
				}

				search = search[len(segment):]
				if search == "/" {
					search = ""
				}
				// the param or the wildcard character was at a leaf
				// we are done with searching

				log.Printf("Will search %s", search)
				continue
			}
			break
		}

		search = search[len(n.prefix):]
	}

	return nil, nil
}






