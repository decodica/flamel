package mage

import (
	"net/http"
	"time"
)

type RequestType int64

//used to tell the server what kind of data to expect
type PayloadType int64

type requestItem int64

const (
	EXTRA requestItem = iota
	HEADER
)

const (
	GET RequestType = iota
	POST
	PUT
	DELETE
)

const (
	HTML PayloadType = iota
	JSON
	XML
	TEXT
	BLOB
)

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

type RequestOutput struct {
	cookies     []*http.Cookie
	headers     map[string]string
	PayloadType PayloadType
	Payload     interface{}
	Error       error
	TplName     string
	Redirect    Redirect
}

func (out *RequestOutput) SetHeader(key string, value string) error {
	out.headers[key] = value
	return nil
}

func (out *RequestOutput) SetCookie(cookie http.Cookie) {
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
	out.SetCookie(cookie);
}

func newRequestOutput(req *http.Request) *RequestOutput {
	out := &RequestOutput{}
	out.headers = make(map[string]string)
	out.PayloadType = TEXT
	out.TplName = base_name
	out.Redirect = Redirect{Status: http.StatusOK, Location: ""}
	return out
}
