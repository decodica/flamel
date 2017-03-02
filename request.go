package mage

import (
	"net/http"
	"time"
)

type requestItem int64

type requestInput struct {
	values      []string
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

type Redirect struct {
	Status   int
	Location string
}

type Renderer interface {
	Render(w http.ResponseWriter) error
}

type RequestOutput struct {
	cookies     []*http.Cookie
	headers     map[string]string
	Renderer    Renderer
}

func newRequestOutput() RequestOutput {
	out := RequestOutput{}
	out.headers = make(map[string]string)
	out.Renderer = &TextRenderer{Data:""};
	return out
}

func (out *RequestOutput) AddHeader(key string, value string) error {
	out.headers[key] = value
	return nil
}

func (out *RequestOutput) AddCookie(cookie http.Cookie) {
	out.cookies = append(out.cookies, &cookie)
}

func (out *RequestOutput) RemoveCookie(name string) {
	index := -1
	expires := time.Unix(0,0);
	for i, v := range out.cookies {
		if v.Name == name {
			c := out.cookies[i];
			c.Value = "";
			c.Expires = expires;
			break
		}
	}

	if index != -1 {
		copy(out.cookies[index:], out.cookies[index+1:])
		out.cookies[len(out.cookies)-1] = nil
		out.cookies = out.cookies[:len(out.cookies)-1]
	}

	cookie := http.Cookie{};
	cookie.Name = name;
	cookie.Expires = expires;
	cookie.MaxAge = 0;
	out.AddCookie(cookie);
}
