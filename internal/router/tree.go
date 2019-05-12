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

	// number of wildcard as direct children
	wildcardCount int

	// number of parameters as direct children
	parameterCount int

	//prefix is the common prefix to ignore
	prefix string

	// sorted slice of edge
	edges edges

	// parent of the node
	parent *node
}

// returns true if the node has parametric children or wildcard
func (n node) isParametrized() bool {
	return n.parameterCount + n.wildcardCount > 0
}

func (n node) params() int {
	return n.parameterCount + n.wildcardCount
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

func (n node) parametricChild() *node {
	if n.parameterCount > 0 {
		return n.getEdge(paramChar)
	}
	return nil
}

func (n node) wildCardChild() *node {
	if n.wildcardCount > 0 {
		return n.getEdge(wildcardChar)
	}
	return nil
}

type tree struct {
	root *node
	size int
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
		params += n.params()
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
				node := node{route: r, prefix: segment, parent: parent}
				e := edge{
					label: segment[0],
					node:  &node,
				}
				switch e.label {
				case paramChar:
					parent.parameterCount++
				case wildcardChar:
					parent.wildcardCount++
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
		child.addEdge(e)
		n.parent = child
		n.prefix = n.prefix[wanted:]

		if n.route != nil {
			switch e.label {
			case paramChar:
				child.parameterCount++
				parent.parameterCount--
			case wildcardChar:
				child.wildcardCount++
				parent.wildcardCount--
			}
		}

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
				child.parameterCount++
			case wildcardChar:
				child.wildcardCount++
			}

			node.parent = child
			child.addEdge(e)
			child = &node
			t.size++
		}

		return child
	}
}

// Finds the requested route
func (t tree) findRoute(search string) (*Route, Params) {
	n := t.root


	// maps all params gathered along the path
	// avoid the use of append
	var params Params
	pcount := 0

	if t.maxArgs > 0 {
		params = make(Params, t.maxArgs)
	}

	for {

		// we traversed all the trie
		// return the route at the node
		if len(search) == 0 {
			if pcount > 0 {
				return n.route, params[:pcount]
			}
			return n.route, nil
		}

		parent := n
		edge := search[0]
		n = n.getEdge(edge)
		// no edge found, route does not exist
		if n == nil || !strings.HasPrefix(search, n.prefix) {
			if parent.isParametrized() {
				// we couldn't find a match, so we go back one level
				// and we check if there's a wildcard or a parameter at the parent level.
				// If so, we walk the wildcard route looking for the correct match.

				// check if we are at the end of the search, assuming no backslashes as route end
				idx := strings.IndexByte(string(search), '/')
				//idx := firstSeparator(search)

				// we are processing the last path segment.
				if idx == -1 {
					// we found a terminal wildcard, i.e. "\example\enzo" with route "\example\*"
					// return the node route
					if n = parent.parametricChild(); n != nil {
						// we found a parameter in the last segment, capture it and return
						p := Param{Key: n.prefix[1:], Value: search}
						params[pcount] = p
						pcount++
						return n.route, params[:pcount]
					}

					if n = parent.wildCardChild(); n != nil {
						if pcount > 0 {
							return n.route, params[:pcount]
						}
						return n.route, nil
					}

					break
				}

				child := parent.parametricChild()

				if child != nil {
					p := Param{Key: child.prefix[1:], Value: search[:idx]}
					params[pcount] = p
					pcount++
				} else {
					child = parent.wildCardChild()
				}

				n = child
				search = search[idx:]
				continue
			}
			break
		}

		search = search[len(n.prefix):]
	}

	return nil, nil
}
