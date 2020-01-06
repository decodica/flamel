package flamel

import (
	"net/http"
	"strings"
)

type ContentOfferer interface {
	DefaultOffer() string
	Offers() []string
}

type defaultContentOfferer struct{}

func (co defaultContentOfferer) DefaultOffer() string {
	return "text/html"
}

func (co defaultContentOfferer) Offers() []string {
	return []string{"text/html", "application/json"}
}

func (f flamel) negotiatedContent(r *http.Request, offerer ContentOfferer) string {
	// parse the accept header
	best := offerer.DefaultOffer()
	if accept, ok := r.Header["Accept"]; ok {
		specs := ParseAccept(accept)
		offers := offerer.Offers()
		wcLevel := 0
		maxQ := -1.0
		for _, offer := range offers {
			for _, spec := range specs {
				switch {
				case spec.Quality == 0.0:
					// ignore the spec
				case spec.Quality < maxQ:
				// skip,w e have better options
				case spec.Value == "*/*":
					if spec.Quality > maxQ && wcLevel <= 1 {
						maxQ = spec.Quality
						best = offer
						wcLevel = 1
					}
				case strings.HasSuffix(spec.Value, "/*"):
					// check if the
					if spec.Quality > maxQ && wcLevel <= 2 && strings.HasPrefix(offer, spec.Value[:len(spec.Value)-2]) {
						maxQ = spec.Quality
						best = offer
						wcLevel = 2
					}
				default:
					if spec.Value == offer && spec.Quality > maxQ {
						maxQ = spec.Quality
						best = offer
						wcLevel = 3
					}
				}
			}
		}

	}

	return best
}
