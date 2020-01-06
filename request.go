package flamel

import (
	"fmt"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"
)

// inputs related definitions and methods
type RequestInputs map[string]requestInput

type requestInput struct {
	files  []*multipart.FileHeader
	values []string
}

func (req requestInput) Multiple() bool {
	return len(req.values) > 1
}

func (req requestInput) Value() string {
	if !req.Multiple() && len(req.values) > 0 {
		return req.values[0]
	}
	return ""
}

func (req requestInput) SetValue(newvalue string) {
	if !req.Multiple() && len(req.values) > 0 {
		req.values[0] = newvalue
	}
}

func (req requestInput) Values() []string {
	return req.values
}

func (req requestInput) Files() []*multipart.FileHeader {
	return req.files
}

type MissingInputError struct {
	key string
}

func (e MissingInputError) Error() string {
	return fmt.Sprintf("missing input for key %s", e.key)
}

// convenience methods to read inputs

func (ins RequestInputs) GetIntWithFormat(key string, base int, size int) (int64, error) {
	val, ok := ins[key]
	if !ok {
		return 0, MissingInputError{key: key}
	}
	return strconv.ParseInt(val.Value(), base, size)
}

func (ins RequestInputs) GetInt(key string) (int64, error) {
	return ins.GetIntWithFormat(key, 10, 64)
}

func (ins RequestInputs) MustInt(key string) int64 {
	i, err := ins.GetInt(key)
	if err != nil {
		panic(err)
	}
	return i
}

// Uint related methods
func (ins RequestInputs) GetUintWithFormat(key string, base int, size int) (uint64, error) {
	val, ok := ins[key]
	if !ok {
		return 0, MissingInputError{key: key}
	}
	return strconv.ParseUint(val.Value(), base, size)
}

func (ins RequestInputs) GetUint(key string) (uint64, error) {
	return ins.GetUintWithFormat(key, 10, 64)
}

func (ins RequestInputs) MustUint(key string) uint64 {
	u, err := ins.GetUint(key)
	if err != nil {
		panic(err)
	}
	return u
}

// float related methods
func (ins RequestInputs) GetFloatWithFormat(key string, size int) (float64, error) {
	val, ok := ins[key]
	if !ok {
		return 0.0, MissingInputError{key: key}
	}
	return strconv.ParseFloat(val.Value(), size)
}

func (ins RequestInputs) GetFloat(key string) (float64, error) {
	return ins.GetFloatWithFormat(key, 64)
}

func (ins RequestInputs) MustFloat(key string) float64 {
	f, err := ins.GetFloat(key)
	if err != nil {
		panic(err)
	}
	return f
}

func (ins RequestInputs) Has(key string) bool {
	_, ok := ins[key]
	return ok
}

func (ins RequestInputs) GetString(key string) (string, error) {
	_, ok := ins[key]
	if !ok {
		return "", MissingInputError{key: key}
	}
	return ins[key].Value(), nil
}

func (ins RequestInputs) MustString(key string) string {
	s, err := ins.GetString(key)
	if err != nil {
		panic(err)
	}
	return s
}

// generic response

type HttpResponse struct {
	Status   int
	Location string
}

// output response

type ResponseOutput struct {
	cookies  []*http.Cookie
	headers  map[string]string
	Renderer Renderer
}

func newResponseOutput() ResponseOutput {
	out := ResponseOutput{}
	out.headers = make(map[string]string)
	out.Renderer = &TextRenderer{Data: ""}
	return out
}

func (out *ResponseOutput) AddHeader(key string, value string) {
	out.headers[key] = value
}

func (out *ResponseOutput) AddCookie(cookie http.Cookie) {
	out.cookies = append(out.cookies, &cookie)
}

func (out *ResponseOutput) SetCookie(cookie http.Cookie) {
	idx := -1
	for i, v := range out.cookies {
		if v.Name == cookie.Name {
			idx = i
			break
		}
	}
	if idx == -1 {
		out.AddCookie(cookie)
		return
	}
	out.cookies[idx] = &cookie
}

func (out *ResponseOutput) RemoveCookie(name string) {
	expires := time.Unix(0, 0)
	for i, v := range out.cookies {
		if v.Name == name {
			c := out.cookies[i]
			c.Value = ""
			c.Expires = expires
			break
		}
	}

	cookie := http.Cookie{}
	cookie.Name = name
	cookie.Expires = expires
	cookie.MaxAge = 0
	out.AddCookie(cookie)
}
