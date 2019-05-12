package router

import (
	"testing"
)


func TestFindRoute(t *testing.T) {
	m := NewRouter()
	m.SetRoute("", nil)
	m.SetRoute("/static", nil)
	m.SetRoute("/static/*",nil)
	m.SetRoute("/static/*/carlo", nil)
	m.SetRoute("/param/:param", nil)
	m.SetRoute("/param/:param/end", nil)
	m.SetRoute("/param/:param/:end",nil)

	mustFind := []string{"/static", "", "/static/wildcard", "/static/wildcard/carlo", "/param/3", "/param/3/end", "/param/3/5"}

	mustFail := []string{"/notexists", "/param/3/2/1", "/param/3/end/5"}

	for _, r := range mustFind {
		route, _ := m.tree.findRoute(r)
		if route == nil {
			t.Fatalf("couldn't find route %s", r)
		}
		t.Logf("found route %s for request %s", route.Name, r)
	}

	for _, r := range mustFail {
		route, _ := m.tree.findRoute(r)
		if route != nil {
			t.Fatalf("should not find route %s for url %s", route.Name, r)
		}
		t.Logf("correctly did not find route %s", r)
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
