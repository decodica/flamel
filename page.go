 package mage

import (
	"html/template"
	"net/http"
	"encoding/json"
	"io"
	"io/ioutil"
	"google.golang.org/appengine"
	"google.golang.org/appengine/blobstore"
	"distudio.com/mage/blueprint"
)

type RequestInputs map[string]requestInput;


type magePage struct {
	query  RequestInputs
	page   Page
	currentUser blueprint.Authenticable
	out *RequestOutput;
}

type Page interface {
	//todo:: OnCreate(context and input)
	//todo: remove draw

	OnCreate(user blueprint.Authenticable);
	Draw() interface{};
	Build(in RequestInputs);
	Process(out *RequestOutput);
	//returns false if the user doesn't require authentication
	//returns true otherwise
	Authorize() Redirect;
	//Called after the response has been set. Used to cleaup resources
	OnFinish();
}

func newGaemsPage(page Page, user blueprint.Authenticable) *magePage {
	p := &magePage{page:page, currentUser:user};
	page.OnCreate(p.currentUser);
	return p;
}

func (page *magePage) build(req *http.Request) {
	reqValues := make(map[string]requestInput);

	//todo: put in context
	method := requestInput{};
	m := make([]string, 1, 1);
	m[0] = req.Method;
	method.values = m;
	reqValues["method"] = method;

	ipV := requestInput{};
	ip := make([]string, 0);
	//todo: how app-engine logs this?
	ip = append(ip, req.RemoteAddr);
	ipV.values = ip;
	reqValues["remote-address"] = ipV;

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
	//todo: put in context
	for k, _ := range req.Header {
		i := requestInput{};
		s := make([]string, 1, 1);
		s[0] = req.Header.Get(k);
		i.values = s;
		reqValues[k] = i;
	}

	//get cookies
	//todo: put in context
	for _, c := range req.Cookies() {
		//threat the auth token apart
		if c.Name == MageInstance().Config.TokenAuthenticationKey {
			continue;
		}
		i := requestInput{};
		s := make([]string, 1, 1);
		s[0] = c.Value;
		i.values = s;
		reqValues[c.Name] = i;
	}

	//get cookie token
	//todo: put in context
	tokenCookie, tokenErr := req.Cookie(MageInstance().Config.TokenAuthenticationKey);
	i := requestInput{};

	if nil == tokenErr {
		s := make([]string, 1, 1);
		s[0] = tokenCookie.Value;
		i.values = s;
	}

	reqValues[(MageInstance().Config.TokenAuthenticationKey)] = i;

	page.query = reqValues;
	page.page.Build(page.query);
}

func (page *magePage) process(req *http.Request) {
	page.out = newRequestOutput(req);

	page.page.Process(page.out);
}

func (page magePage) input(key string) (requestInput, bool) {
	v, ok := page.query[key];
	return v, ok;
}

func (page *magePage) output(w http.ResponseWriter) {

	if page.out.Error != nil {
		//todo sostituire con template
		panic(page.out.Error);
	}

	drawable := page.page.Draw();

	switch (page.out.PayloadType) {
		case HTML:
			t, ok := drawable.(*template.Template);
			if !ok {
				http.Error(w, "Invalid template", http.StatusInternalServerError );
				return;
			}
			err := t.ExecuteTemplate(w, page.out.TplName, page.out.Payload)
			if err != nil {
				panic(err);
			}
		break;
		case JSON:
			json.NewEncoder(w).Encode(page.out.Payload);
		break;
		case TEXT:
			str, ok := page.out.Payload.(string);
			if !ok {
				return;
			}
			io.WriteString(w, str);
		break;
		case BLOB:
			key, ok := page.out.Payload.(appengine.BlobKey);
			if !ok {
				//todo: panic
				return;
			}
			blobstore.Send(w, key);
		break;
	}
}

func (page *magePage) finish() {
	page.page.OnFinish();
}
