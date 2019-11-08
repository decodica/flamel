package flamel

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNegotiation_Parse(t *testing.T) {
	fl := flamel{}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	//req.Header.Add("Accept", "text/html, application/xhtml+xml, application/xml;q=0.9, image/webp, */*;q=0.8")

	req.Header.Add("Accept", "text/csv")
	det := fl.negotiatedContent(req, defaultContentOfferer{})

	if det != "text/html" {
		t.Fail()
	}
}

func BenchmarkNegotiation(b *testing.B) {
	fl := flamel{}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Add("Accept", "text/html, application/xhtml+xml, application/xml;q=0.9, image/webp, */*;q=0.8")

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		fl.negotiatedContent(req, defaultContentOfferer{})
	}
}
