package router

import (
	"bytes"
	"testing"
)

func TestFindRoute(t *testing.T) {
	m := NewRouter()
	m.SetRoute("", nil)
	m.SetRoute("/:param", nil)
	m.SetRoute("/static", nil)
	m.SetRoute("/stai", nil)
	m.SetRoute("/static/*", nil)
	m.SetRoute("/static/*/:param", nil)
	m.SetRoute("/param/:param", nil)
	m.SetRoute("/param/:param/end", nil)
	m.SetRoute("/param/:param/:end", nil)
	m.SetRoute("/param/first/second", nil)
	m.SetRoute("/param/*", nil)

	// map requests with expected route
	mustFind := map[string]string{
		"/antani": "/:param",
		"/static": "/static",
		"/station": "/:param",
		"": "",
		"/static/wildcard": "/static/*",
		"/static/wildcard/3": "/static/*/:param",
		"/static/wildcard/first/second/third": "/static/*",
		"/param": "/:param",
		"/param/3": "/param/:param",
		"/param/3/end": "/param/:param/end",
		"/param/3/5": "/param/:param/:end",
		"/param/first/second": "/param/first/second",
		"/param/3/end/wildcard": "/param/*",
	}

	mustFail := []string{
		"/none/3/2/1",
		"/stai/con/me",
	}

	for r, _ := range mustFind {
		route, params := m.tree.findRoute(r)
		if route == nil {
			t.Fatalf("couldn't find route %s", r)
		}

		var builder bytes.Buffer

		count := 0
		for _, p := range params {
			count++
			builder.WriteString(p.Key)
			builder.WriteString(" = ")
			builder.WriteString(p.Value)
			builder.WriteString(", ")
		}

		if mustFind[r] == route.Name {
			if count == 0 {
				t.Logf("found route %s for request %s with no params", route.Name, r)
				_ = builder.String()
			} else {
				t.Logf("found route %s for request %s with params %s", route.Name, r, builder.String())
			}

		} else {
			t.Fatalf("should not find route %s for request %s", route.Name, r)
		}


	}

	for _, r := range mustFail {
		route, _ := m.tree.findRoute(r)
		if route != nil {
			t.Fatalf("should not find route %s for url %s", route.Name, r)
		}
		t.Logf("correctly did not find route %s", r)
	}
}

func TestMaxParams(t *testing.T) {
	m := NewRouter()
	m.SetRoute("", nil)
	m.SetRoute("/*", nil)
	m.SetRoute("/:param", nil)
	m.SetRoute("/three/:first/:second/:third", nil)
	m.SetRoute("/three/first/second/third/:fourth", nil)

	if m.tree.maxArgs != 3 {
		t.Fatalf("couldn't determine max params. Found %d max params instead of 3", m.tree.maxArgs)
	}
}

func BenchmarkGetEdge(b *testing.B) {
	r := NewRouter()
	r.SetRoute("/first/:first/second/:second/third/:third/fourth/:fourth/fifth/:fifth", nil)
	b.ReportAllocs()
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		r.tree.root.getEdge('/')
	}
}

func BenchmarkFindRoute_static(b *testing.B) {
	r := NewRouter()
	r.SetRoute("/first/second/third", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		r.tree.findRoute("/first/second/third")
	}
}

func BenchmarkFindRoute_five(b *testing.B) {
	r := NewRouter()
	r.SetRoute("/first/:first/second/:second/third/:third/fourth/:fourth/fifth/:fifth", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		r.tree.findRoute("/first/1/second/2/third/3/fourth/4/fifth/5")
	}
}
