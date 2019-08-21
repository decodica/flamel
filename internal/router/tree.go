package router

import (
	"sort"
	"strings"
)

// A specialized radix tree implementation to handle route matching.
// heavily inspired by @https://github.com/armon/go-radix/blob/master/radix.go
// a route is our leaf node, where route name is the key.
// Differently from a pure radix tree, on insertion all path segments are created if they do not exist
// ex: inserting only the node at "/my/route/example" creates six nodes, separated by '/'
// namely: "/", "my", "/", "route", "/", "example"

// An edge connects one node with another in a parent->child relation
// The label is the byte connecting each node and it coincides with the first character
// of the child node
type edge struct {
	label byte
	node  *node
}

type edges []edge

func (e edges) Len() int {
	return len(e)
}

// edges implement the sortable interface
func (e edges) Less(i, j int) bool {
	return e[i].label < e[j].label
}

func (e edges) Swap(i, j int) {
	e[i], e[j] = e[j], e[i]
}

func (e edges) Sort() {
	sort.Sort(e)
}

// The node of the tree
type node struct {
	// route associated with the node. It can be nil
	route *Route

	//prefix is the common prefix to ignore
	prefix string

	// sorted slice of edge
	edges edges

	// parent of the node
	parent *node

	// wildcard
	wildcardChild *node

	// param
	paramChild *node

	// true if the node is parametric or a wildcard
	isParametric bool
}

// returns true if the node has parametric children or wildcard
func (n node) hasParametricChildren() bool {
	return n.wildcardChild != nil || n.paramChild != nil
}

func (n *node) addEdge(edge edge) {
	n.edges = append(n.edges, edge)
	edge.node.parent = n
	n.edges.Sort()
}

func (n *node) updateEdge(label byte, node *node) {
	count := len(n.edges)
	idx := sort.Search(count, func(i int) bool {
		return n.edges[i].label >= label
	})

	if idx < count && n.edges[idx].label == label {
		n.edges[idx].node = node
		node.parent = n
		return
	}

	panic("update on missing edge")
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

func (n node) isLeaf() bool {
	return len(n.edges) == 0
}

type tree struct {
	root    *node
	size    int
	maxArgs int
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
	return i
}

// explode the compressed note by creating a new edge for each extra url
// for a given url "/first/second/third" we add the edges: "/", "first", "/", "second", "/", "third"
func splitSegments(url string) []string {

	segments := make([]string, 0)
	for len(url) > 0 {
		idx := strings.IndexByte(url, '/')
		if idx == -1 {
			segments = append(segments, url)
			break
		}

		if idx == 0 {
			segments = append(segments, string(url[0]))
			url = url[1:]
			continue
		}

		s, slash := url[:idx], url[idx:idx+1]
		segments = append(segments, s, slash)
		url = url[idx+1:]
	}

	return segments
}

func (t *tree) insert(route *Route) {
	n := t.addEdge(route)
	// walk the tree and count all the path params
	params := 0
	for n.parent != nil {
		if n.isParametric {
			params++
		}
		n = n.parent
	}

	if params > t.maxArgs {
		t.maxArgs = params
	}
}

// adds a new node or updates an existing one
// returns true if the node has been updated
func (t *tree) addEdge(route *Route) *node {

	var parent *node
	n := t.root
	search := route.Name

	for {
		if len(search) == 0 {
			// we append the route at the end of the tree.
			n.route = route

			// if we are not at the leaf, we increment the tree size
			t.size++

			return n
		}

		// look for the edge
		parent = n
		n = n.getEdge(search[0])
		// there is no edge from the parent to the new node.
		// we create a new edge and a new node, using the search as prefix
		// and we connect it to the new node (parent)-----(new-node)
		// or we have an empty tree
		if n == nil {
			segments := splitSegments(search)
			l := len(segments)

			for i := 0; i < l; i++ {
				var r *Route
				if i == l-1 {
					r = route
				}

				segment := segments[i]
				node := node{route: r, prefix: segment}
				e := edge{
					label: segment[0],
					node:  &node,
				}
				switch segment[0] {
				case paramChar:
					parent.paramChild = &node
					node.isParametric = true
				case wildcardChar:
					parent.wildcardChild = &node
				}

				parent.addEdge(e)

				parent = &node
				t.size++
			}
			return parent
		}

		// we found an edge to attach the new node
		// common is the idx of the divergent char
		// i.e. "aab" and "aac" then common has value 2
		wanted := longestPrefix(search, n.prefix)

		// if the prefixes coincide in len
		// we consume the search and continue the loop with the remaining slice.
		// we have this case when ex.confronting /static with /static/enzo. In this case the common chars
		// are equal to the node prefix (/static).
		// We walk the node and look for a place to append the route following this path
		if wanted == len(n.prefix) {
			search = search[wanted:]
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

		// now that we split the nodes, we re-prepend the newly created node (created from the split)
		// to the common part.
		// ex. we are inserting "/static/enzo" and we find "/static/carlo"
		// in this case we create a new node with prefix "/static/", we append the "carlo" to the "/static" node
		// and we add "enzo" to the static node
		// we must update the state of the wildcardChild, checking if any wildcard are left
		e := edge{
			label: n.prefix[wanted],
			node:  n,
		}

		n.prefix = n.prefix[wanted:]

		switch e.label {
		case paramChar:
			child.paramChild = n
			n.isParametric = true
		case wildcardChar:
			child.wildcardChild = n
		}

		child.addEdge(e)

		search = search[wanted:]
		// If the new key was the same of the parent
		// we assign the route to the node.
		if len(search) == 0 {
			child.route = route
			return child
		}

		// if the prefix contains two or more segments of the url, we break it into multiple
		// empty nodes
		segments := splitSegments(search)
		l := len(segments)

		for i := 0; i < l; i++ {
			var r *Route
			if i == l-1 {
				r = route
			}
			segment := segments[i]
			node := node{route: r, prefix: segment}
			e = edge{
				label: segment[0],
				node:  &node,
			}
			switch e.label {
			case paramChar:
				child.paramChild = &node
				node.isParametric = true
			case wildcardChar:
				child.wildcardChild = &node
			}

			child.addEdge(e)

			child = &node
			t.size++
		}

		return child
	}
}

// Finds the requested route
func (t tree) findRoute(wanted string) (*Route, Params) {
	n := t.root

	search := wanted
	// maps all params gathered along the path
	// avoid the use of append
	var params Params
	pcount := 0

	if t.maxArgs > 0 {
		params = make(Params, t.maxArgs)
	}

	// lp is the last parametric node we encountered
	var lp *node

	var wild *node

	// freeze index in case of parameter
	tag := 0

	for {

		var edge byte

		// we traversed all the trie
		// return the route at the node
		slen := len(search)

		if slen == 0 {
			if n.route == nil && lp != nil {
				n = lp.paramChild
			} else {
				return n.route, params[:pcount]
			}
		} else {
			edge = search[0]
		}

		if edge == '/' {
			lp = nil
		}

		n = n.getEdge(edge)

		if n != nil && n.hasParametricChildren() {
			lp = n
			if w := lp.wildcardChild; w != nil {
				wild = w
			}
			// freeze the param value to the moment we find a parameter, in case we need to go back
			tag = len(wanted) - slen + 1
		}

		// we arrived at the end of the selected path
		if n == nil || slen < len(n.prefix) || search[0:len(n.prefix)] != n.prefix {

			// there's no valid parametric node on the way, we can break the loop, the search is over
			if lp == nil {
				break
			}

			// else, we must reconsider our path by walking a parametric route: backtracking
			// we couldn't find a match, so we go back to the last parametric node we found on the way.
			// If such a node exists, we walk the parametric route looking for the correct match.

			until := strings.IndexByte(wanted[tag:], '/')

			// we are at the end of the string,
			if until == -1 {
				until = len(wanted) - tag
			}

			if n = lp.paramChild; n != nil {
				params[pcount].Key = n.prefix[1:]
				params[pcount].Value = wanted[tag : tag+until]
				pcount++
			} else {
				n = lp.wildcardChild
			}

			lp = nil
			search = wanted[tag+until:]
			continue
		}

		search = search[len(n.prefix):]
	}

	if wild != nil {
		return wild.route, params[:pcount]
	}

	return nil, nil
}
