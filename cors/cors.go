package cors

import (
	"fmt"
	"net/http"
)

type Cors struct {
	headers string;
	methods string;
	origins string;
	//seconds to cache the response
	MaxAgeSeconds int;
}

func NewCors(origins []string, methods []string, headers []string) *Cors {

	c := Cors{};
	c.headers = convertToHeaderString(headers);
	c.methods = convertToHeaderString(methods);
	c.origins = convertToHeaderString(origins);

	return &c;
}

func (c *Cors) HandleOptions(w http.ResponseWriter) http.ResponseWriter {
	if c.origins != "" {
		w.Header().Set("Access-Control-Allow-Origin", c.origins);
	}

	if c.methods != "" {
		w.Header().Set("Access-Control-Allow-Methods", c.methods);
	}

	if c.headers != "" {
		w.Header().Set("Access-Control-Allow-Headers", c.headers);
	}

	if c.MaxAgeSeconds > 0 {
		age := fmt.Sprintf("%d", c.MaxAgeSeconds);
		w.Header().Set("Access-Control-Max-Age", age);
	}

	return w;
}

func convertToHeaderString(values []string) string {

	s := "";
	for i, v := range values {
		if i == 0 {
			s = fmt.Sprintf("%s", v);
			continue;
		}

		s = fmt.Sprintf("%s, %s", s, v);
	}

	return s;
}


