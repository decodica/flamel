package flamel

import (
	"strconv"
	"strings"
)

type char byte

var chars [256]char

const (
	isToken char = 1 << iota
	isSpace
)

func init() {
	buildChars()
}

func buildChars() {
	// we define the charset according to RFC 2616
	// see https://github.com/golang/gddo/blob/master/httputil/header/header.go
	for c := 0; c < 256; c++ {
		var v char
		// CTL: any US-ASCII control characters (octs o to 31 including DEL - 127 -)
		isCtl := c <= 31 || c == 127
		// CHAR: any US-ASCII character (octects 0 to 127)
		isChar := c <= 127

		// we check if the char is a separator
		isSeparator := strings.IndexRune(" \t\"(),/:;<=>?@[]\\{}", rune(c)) >= 0

		// we check if the char is whitespace
		if strings.IndexRune(" \t\r\n", rune(c)) >= 0 {
			v |= isSpace
		}

		// a token is defined as a character that is not a control character nor a separator
		if isChar && !isCtl && !isSeparator {
			v |= isToken
		}
		chars[c] = v
	}
}

// parser general methods

// retrieves a token used in accept header values, i.e. "text/html"
func extractAcceptToken(s string) (string, string) {
	i := 0
	for ; i < len(s); i++ {
		v := s[i]
		// we have a valid token if we have tokens including the slash character
		if chars[v]&isToken == 0 && v != '/' {
			break
		}
	}
	return s[:i], s[i:]
}

// removes spaces from a given string
func skipSpace(s string) string {
	for i := 0; i < len(s); i++ {
		v := s[i]
		if chars[v]&isSpace == 0 {
			return s[i:]
		}
	}
	return s
}

func extractQuality(s string) (float64, string) {
	if len(s) == 0 {
		return -1, ""
	}

	i := 0
	for ; i < len(s); i++ {
		v := s[i]
		// we admit a point "." only in second position since the quality score goes from 0 to 1
		if !(i == 1 && v == '.') && v < '0' || v > '9' {
			break
		}
	}

	q, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return -1, s[i:]
	}

	if q > 1 {
		return -1, s[i:]
	}

	return q, s[i:]
}

type contentSpecification struct {
	Quality float64
	Value   string
}

// parses the "Accept" header value. e.g: "text/html, application/xhtml+xml, application/xml;q=0.9, image/webp, */*;q=0.8"
func ParseAccept(accept []string) []contentSpecification {
	return parseAcceptFormat(accept)
}

func parseAcceptFormat(values []string) []contentSpecification {
	var specs []contentSpecification
	for _, s := range values {
		for {
			var spec contentSpecification
			spec.Value, s = extractAcceptToken(s)
			if spec.Value == "" {
				break
			}
			spec.Quality = 1.0
			s = skipSpace(s)

			// we are done with the format specification path
			if strings.HasPrefix(s, ";") {
				s = skipSpace(s[1:])
				//
				if !strings.HasPrefix(s, "q=") {
					break
				}
				spec.Quality, s = extractQuality(s[2:])
				if spec.Quality < 0.0 {
					break
				}
			}

			specs = append(specs, spec)
			s = skipSpace(s)
			if !strings.HasPrefix(s, ",") {
				break
			}
			s = skipSpace(s[1:])
		}
	}

	return specs
}
