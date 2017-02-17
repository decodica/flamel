 package mage

import (
	"net/http"
	"io/ioutil"
	"golang.org/x/net/context"
)

type RequestInputs map[string]requestInput;


type magePage struct {
	page        Page
	out         RequestOutput;
}

type Page interface {
	//the page logic is executed here
	//if the user is valid it is recoverable from the context
	//else the user is nil
	Process(ctx context.Context, out *RequestOutput) Redirect;
	//called to release resources
	OnDestroy(ctx context.Context);
}

func newGaemsPage(page Page) *magePage {
	p := &magePage{page:page};
	return p;
}

func (page *magePage) build(ctx context.Context, req *http.Request) context.Context {

	reqValues := make(map[string]requestInput);

	//todo: put in context
	method := requestInput{};
	m := make([]string, 1, 1);
	m[0] = req.Method;
	method.values = m;
	reqValues[REQUEST_METHOD] = method;


	ipV := requestInput{};
	ip := make([]string, 0);
	//todo: how app-engine logs this?
	ip = append(ip, req.RemoteAddr);
	ipV.values = ip;
	reqValues[REQUEST_IPV4] = ipV;

	//get request params
	switch req.Method {
		case "GET":
			for k, _ := range req.URL.Query() {
				s := make([]string, 1, 1);
				v := req.URL.Query().Get(k);
				s[0] = v;
				i := requestInput{};
				i.values = s;
				reqValues[k] = i;
			}
			break;
		case "POST":
			req.ParseForm();
			reqType := req.Header.Get("Content-Type");
			if reqType == "application/json" {
				s := make([]string, 1, 1);
				i := requestInput{};
				str, _ := ioutil.ReadAll(req.Body);
				s[0] = string(str);
				i.values = s;
				reqValues["json"] = i;
				break;
			}

			for k , _ := range req.Form {
				v := req.Form[k];
				i := requestInput{};
				i.values = v;
				reqValues[k] = i;
			}
		case "PUT":
			req.ParseForm();
			for k , _ := range req.Form {
				v := req.Form[k];
				i := requestInput{};
				i.values = v;
				reqValues[k] = i;
			}
		break;
		case "DELETE":
			for k, _ := range req.URL.Query() {
				s := make([]string, 1, 1);
				v := req.URL.Query().Get(k);
				s[0] = v;
				i := requestInput{};
				i.values = s;
				reqValues[k] = i;
			}
			break;
		default:
	}

	//get the headers
	for k, _ := range req.Header {
		i := requestInput{};
		s := make([]string, 1, 1);
		s[0] = req.Header.Get(k);
		i.values = s;
		reqValues[k] = i;
	}

	//get cookies
	for _, c := range req.Cookies() {
		//threat the auth token apart
		i := requestInput{};
		s := make([]string, 1, 1);
		s[0] = c.Value;
		i.values = s;
		reqValues[c.Name] = i;
	}


	return context.WithValue(ctx, REQUEST_INPUTS, reqValues);
}

func (page *magePage) process(ctx context.Context, req *http.Request) Redirect {
	//create and package the request
	page.build(ctx, req);

	page.out = newRequestOutput();
	return page.page.Process(ctx, &page.out);
}
