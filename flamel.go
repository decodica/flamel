package flamel

import (
	"bytes"
	"context"
	"decodica.com/flamel/cors"
	"decodica.com/flamel/internal/router"
	"fmt"
	"google.golang.org/appengine"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
)

type flamel struct {
	Config
	app            Application
	bufferPool     *sync.Pool
	services       []Service
	contentOfferer ContentOfferer
}

type Application interface {
	// called as soon as the request is received and the context is created
	OnStart(ctx context.Context) context.Context
	// called after each response has been finalized
	AfterResponse(ctx context.Context)
}

func (fl *flamel) AddService(service Service) {
	fl.services = append(fl.services, service)
}

type Config struct {
	//true if the server suport Cross Origin Request
	CORS                    *cors.Cors
	EnforceHostnameRedirect string
	MaxFileUploadSize       int64
	ContentOfferer          ContentOfferer
	Router
}

func DefaultConfig() Config {
	config := Config{}
	config.Router = NewDefaultRouter()
	// default max size of upload is 4 megs
	config.MaxFileUploadSize = (1 << 20) * 4
	config.ContentOfferer = defaultContentOfferer{}
	return config
}

const (
	//request related special vars
	KeyRequestInputs     = "__flamel_request_inputs__"
	KeyRequestMethod     = "__flamel__method__"
	KeyRequestIPV4       = "__flamel_remote_address__"
	KeyRequestJSON       = "__flamel_json__"
	KeyRequestURL        = "__flamel_URL__"
	KeyRequestScheme     = "__flamel_scheme__"
	KeyRequestQuery      = "__flamel_query__"
	KeyRequestHost       = "__flamel_host__"
	KeyNegotiatedContent = "__flamel_negotiated_content__"
)

var instance *flamel
var once sync.Once

//singleton instance
func Instance() *flamel {

	once.Do(func() {
		config := DefaultConfig()

		pool := sync.Pool{
			New: func() interface{} {
				return bytes.Buffer{}
			},
		}
		instance = &flamel{Config: config, bufferPool: &pool, contentOfferer: config.ContentOfferer}
	})

	return instance
}

func (fl *flamel) Run(application Application) {
	fl.launchApp(application)
	defer fl.end()
	http.HandleFunc("/", fl.run)
	appengine.Main()
}

func (fl *flamel) end() {
	for _, s := range fl.services {
		s.Destroy()
	}
}

func (fl *flamel) launchApp(application Application) {
	if fl.app != nil {
		panic("Application already set")
	}
	fl.app = application

	// initialize services
	for _, s := range fl.services {
		s.Initialize()
	}
}

func (fl *flamel) run(w http.ResponseWriter, req *http.Request) {

	if fl.app == nil {
		panic("must set Flamel's application!")
	}

	ctx := appengine.NewContext(req)

	ctx = fl.app.OnStart(ctx)
	for _, s := range fl.services {
		ctx = s.OnStart(ctx)
	}

	//if we enforce the hostname and the request hostname doesn't match, we redirect to the requested host
	//host is in the form domainname.com
	if fl.Config.EnforceHostnameRedirect != "" && fl.Config.EnforceHostnameRedirect != req.Host {
		scheme := "http"
		if req.URL.Scheme == "https" {
			scheme = "https"
		}

		hst := fmt.Sprintf("%s://%s%s", scheme, fl.Config.EnforceHostnameRedirect, req.URL.Path)

		if req.URL.RawQuery != "" {
			hst = fmt.Sprintf("%s?%s", hst, req.URL.RawQuery)
		}

		http.Redirect(w, req, hst, http.StatusMovedPermanently)
		renderer := TextRenderer{}
		renderer.Render(w)
		return
	}

	origin := req.Header.Get("Origin")
	hasOrigin := origin != ""

	//handle CORS requests
	if hasOrigin && fl.Config.CORS != nil && req.Method == http.MethodOptions {
		fl.Config.CORS.HandleOptions(w, origin)
		w.Header().Set("Content-Type", "text/html; charset=utf8")
		renderer := TextRenderer{}
		renderer.Render(w)
		return
	}

	//add inputs to the context object
	ins, err := fl.parseRequestInputs(ctx, req)
	ctx = context.WithValue(ctx, KeyRequestInputs, ins)
	if err != nil {
		renderer := TextRenderer{}
		renderer.Data = err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		renderer.Render(w)
		return
	}

	ctx, err, controller := fl.RouteForPath(ctx, req.URL.Path)

	if err == router.ErrRouteNotFound {
		renderer := TextRenderer{}
		renderer.Data = err.Error()
		w.WriteHeader(http.StatusNotFound)
		renderer.Render(w)
		return
	}

	if err != nil {
		renderer := TextRenderer{}
		renderer.Data = err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		renderer.Render(w)
		return
	}

	defer fl.destroy(ctx, controller)

	// negotiated content
	cnt := requestInput{}
	if offerer, ok := controller.(ContentOfferer); ok {
		cnt.values = []string{fl.negotiatedContent(req, offerer)}
	} else {
		cnt.values = []string{fl.negotiatedContent(req, fl.contentOfferer)}
	}

	ins[KeyNegotiatedContent] = cnt

	out := newResponseOutput()

	//handle the CORS framework
	if fl.Config.CORS != nil {

		//handle the AMP case
		if fl.Config.CORS.AMPForUrl(req.URL.Path) {
			AMPsource, hasSource := req.URL.Query()[cors.KeyAmpSourceOrigin]

			//if the source is not set the AMP request is invalid
			if !hasSource || len(AMPsource) == 0 {
				w.WriteHeader(http.StatusNotAcceptable)
				return
			}

			//if the source is set we check if the AMP-Same-Origin is set
			hasSameOrigin := req.Header.Get(cors.KeyAmpSameOriginHeader) == "true"

			if !(hasOrigin || hasSameOrigin) {
				w.WriteHeader(http.StatusNotAcceptable)
				return
			}

			//if the value of AMP_SAME_ORIGIN is different from true we validate the origin
			//amongst those accepted
			if fl.Config.CORS.ValidateAMP(w, AMPsource[0]) != nil {
				w.WriteHeader(http.StatusNotAcceptable)
				return
			}
		}

		if hasOrigin {
			allowed := fl.Config.CORS.HandleOptions(w, origin)
			if !allowed {
				w.WriteHeader(http.StatusForbidden)
				return
			}
		}
	}

	response := controller.Process(ctx, &out)

	//add headers and cookies
	for _, v := range out.cookies {
		http.SetCookie(w, v)
	}

	for k, v := range out.headers {
		w.Header().Set(k, v)
	}

	if response.Status >= 300 && response.Status < 400 {
		http.Redirect(w, req, response.Location, response.Status)
	}

	if response.Status >= 400 {
		w.WriteHeader(response.Status)
	}

	err = out.Renderer.Render(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (fl *flamel) destroy(ctx context.Context, controller Controller) {
	controller.OnDestroy(ctx)
	controller = nil
	for _, s := range fl.services {
		s.OnEnd(ctx)
	}
	fl.app.AfterResponse(ctx)
}

func (fl flamel) parseRequestInputs(ctx context.Context, req *http.Request) (RequestInputs, error) {
	reqValues := make(RequestInputs)

	reqValues[KeyRequestHost] = requestInput{
		values: []string{req.URL.Host},
	}

	reqValues[KeyRequestScheme] = requestInput{
		values: []string{req.URL.Scheme},
	}

	reqValues[KeyRequestQuery] = requestInput{
		values: []string{req.URL.RawQuery},
	}

	reqValues[KeyRequestURL] = requestInput{
		values: []string{req.URL.Path},
	}

	reqValues[KeyRequestMethod] = requestInput{
		values: []string{req.Method},
	}

	reqValues[KeyRequestIPV4] = requestInput{
		values: []string{req.RemoteAddr},
	}

	//get request params
	switch req.Method {
	case http.MethodDelete:
		fallthrough
	case http.MethodGet:
		for k, v := range req.URL.Query() {
			i := requestInput{}
			i.values = v
			reqValues[k] = i
		}
		break
	case http.MethodPut:
		fallthrough
	case http.MethodPost:
		reqType := req.Header.Get("Content-Type")
		// parse the multiform data if the request specifies it as its content type
		if strings.Contains(reqType, "multipart/form-data") {
			if err := req.ParseMultipartForm(fl.MaxFileUploadSize); err != nil {
				return nil, err
			}

			// add the filehandles to the context
			for k, v := range req.MultipartForm.File {
				i := requestInput{}
				i.files = v
				reqValues[k] = i
			}
		} else {
			err := req.ParseForm()
			if err != nil {
				return nil, err
			}
		}

		if strings.Contains(reqType, "application/json") {
			s := make([]string, 1, 1)
			i := requestInput{}
			str, _ := ioutil.ReadAll(req.Body)
			s[0] = string(str)
			i.values = s
			reqValues[KeyRequestJSON] = i
			break
		}

		for k, v := range req.Form {
			i := requestInput{}
			i.values = v
			reqValues[k] = i
		}
	}

	//get the headers
	for k := range req.Header {
		i := requestInput{}
		s := make([]string, 1, 1)
		s[0] = req.Header.Get(k)
		i.values = s
		reqValues[k] = i
	}

	//get cookies
	for _, c := range req.Cookies() {
		//threat the auth token apart
		i := requestInput{}
		s := make([]string, 1, 1)
		s[0] = c.Value
		i.values = s
		reqValues[c.Name] = i
	}

	return reqValues, nil
}
