package flamel

import (
	"mime/multipart"
	"net/http"
	"time"
)

type RequestInputs map[string]requestInput

type requestInput struct {
	files []*multipart.FileHeader
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

type Redirect struct {
	Status   int
	Location string
}

type Renderer interface {
	Render(w http.ResponseWriter) error
}

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
