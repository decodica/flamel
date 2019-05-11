package mage

import (
	"bytes"
	"distudio.com/mage/cors"
	"distudio.com/mage/internal/router"
	"errors"
	"fmt"
	"golang.org/x/net/context"
	"google.golang.org/appengine"
	"google.golang.org/appengine/blobstore"
	"google.golang.org/appengine/file"
	"google.golang.org/appengine/image"
	"google.golang.org/appengine/memcache"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
)

type mage struct {
	Config
	app        Application
	bufferPool *sync.Pool
}

type Application interface {
	//called as soon as the request is received and the context is created
	OnStart(ctx context.Context) context.Context
	//called after each response has been finalized
	AfterResponse(ctx context.Context)
}

func (mage *mage) LaunchApp(application Application) {
	if mage.app != nil {
		panic("Application already set")
	}
	mage.app = application
}

type Config struct {
	UseMemcache       bool
	MaxFileUploadSize int64
	//true if the server suport Cross Origin Request
	CORS                    *cors.Cors
	EnforceHostnameRedirect string
	Router
}

func DefaultConfig() Config {
	config := Config{}
	config.Router = NewDefaultRouter()
	config.MaxFileUploadSize = SettingDefaultMaxFileSize
	return config
}

const (
	//request related special vars
	KeyRequestInputs = "request-inputs"
	KeyRequestMethod = "method"
	KeyRequestIPV4   = "remote-address"
	KeyRequestJSON   = "__mage_json__"
	KeyRequestURL    = "__mage_URL__"
	KeyRequestScheme = "__mage_Scheme__"

	//default at 4 megs
	SettingDefaultMaxFileSize = (1 << 20) * 4
)

//mage is a singleton
var instance *mage
var once sync.Once

//for now, cache the blobkey in memcache as key/value pair.
//TODO: create a corrispondency "table" with imageName/key in upload/update

func ImageServingUrlString(c context.Context, bucket string, fileName string) (string, error) {

	if fileName == "" {
		return "", errors.New("filename must not be empty")
	}

	if bucket == "" {
		b, err := file.DefaultBucketName(c)
		if err != nil {
			return "", err
		}
		bucket = b
	}


	//get the urlString from cache
	item, err := memcache.Get(c, fileName)

	if err == memcache.ErrCacheMiss {

		fullName := "/gs/" + bucket + "/" + fileName
		key, err := blobstore.BlobKeyForFile(c, fullName)

		if err != nil {
			return "", err
		}

		url, err := image.ServingURL(c, key, nil)

		if err != nil {
			return "", err
		}

		item := &memcache.Item{}
		item.Key = fileName
		item.Value = []byte(url.String())
		err = memcache.Set(c, item)
		if err != nil {
			panic(err)
		}

		return url.String(), err
	}

	return string(item.Value), err
}

//singleton instance
func Instance() *mage {

	once.Do(func() {
		config := DefaultConfig()

		pool := sync.Pool{
			New: func() interface{} {
				return bytes.Buffer{}
			},
		}
		instance = &mage{Config: config, bufferPool: &pool}
	})

	return instance
}

func (mage *mage) Run(w http.ResponseWriter, req *http.Request) {

	if mage.app == nil {
		panic("Must set MAGE Application!")
	}

	ctx := appengine.NewContext(req)

	ctx = mage.app.OnStart(ctx)

	//if we enforce the hostname and the request hostname doesn't match, we redirect to the requested host
	//host is in the form domainname.com
	if mage.Config.EnforceHostnameRedirect != "" && mage.Config.EnforceHostnameRedirect != req.Host {
		scheme := "http"
		if req.URL.Scheme == "https" {
			scheme = "https"
		}

		hst := fmt.Sprintf("%s://%s%s", scheme, mage.Config.EnforceHostnameRedirect, req.URL.Path)

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
	if hasOrigin && mage.Config.CORS != nil && req.Method == http.MethodOptions {
		mage.Config.CORS.HandleOptions(w, origin)
		w.Header().Set("Content-Type", "text/html; charset=utf8")
		renderer := TextRenderer{}
		renderer.Render(w)
		return
	}

	//add inputs to the context object
	ctx, err := mage.parseRequestInputs(ctx, req)
	if err != nil {
		renderer := TextRenderer{}
		renderer.Data = err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		renderer.Render(w)
		return
	}

	ctx, err, controller := mage.RouteForPath(ctx, req.URL.Path)

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

	defer mage.destroy(ctx, controller)
	out := newResponseOutput()

	//handle the CORS framework
	if mage.Config.CORS != nil {

		//handle the AMP case
		if mage.Config.CORS.AMPForUrl(req.URL.Path) {
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
			if mage.Config.CORS.ValidateAMP(w, AMPsource[0]) != nil {
				w.WriteHeader(http.StatusNotAcceptable)
				return
			}
		}

		if hasOrigin {
			allowed := mage.Config.CORS.HandleOptions(w, origin)
			if !allowed {
				w.WriteHeader(http.StatusForbidden)
				return
			}
		}
	}

	redirect := controller.Process(ctx, &out)

	//add headers and cookies
	for _, v := range out.cookies {
		http.SetCookie(w, v)
	}

	for k, v := range out.headers {
		w.Header().Set(k, v)
	}

	if redirect.Status >= 300 && redirect.Status < 400 {
		http.Redirect(w, req, redirect.Location, redirect.Status)
	}

	if redirect.Status >= 400 {
		w.WriteHeader(redirect.Status)
	}

	err = out.Renderer.Render(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (mage *mage) destroy(ctx context.Context, controller Controller) {
	controller.OnDestroy(ctx)
	controller = nil
	mage.app.AfterResponse(ctx)
}

func (mage mage) parseRequestInputs(ctx context.Context, req *http.Request) (context.Context, error) {
	reqValues := make(RequestInputs)

	reqValues[KeyRequestScheme] = requestInput{
		values: []string{req.URL.Scheme},
	}

	reqValues[KeyRequestURL] = requestInput{
		values:[]string{req.URL.Path},
	}

	reqValues[KeyRequestMethod] = requestInput{
		values:[]string{req.Method},
	}

	reqValues[KeyRequestIPV4] = requestInput{
		values:[]string{req.RemoteAddr},
	}

	//get request params
	switch req.Method {
	case http.MethodDelete:
		fallthrough
	case http.MethodGet:
		for k := range req.URL.Query() {
			s := make([]string, 1, 1)
			v := req.URL.Query().Get(k)
			s[0] = v
			i := requestInput{}
			i.values = s
			reqValues[k] = i
		}
		break
	case http.MethodPut:
		fallthrough
	case http.MethodPost:
		reqType := req.Header.Get("Content-Type")
		//parse the multiform data if the request specifies it as its content type
		if strings.Contains(reqType, "multipart/form-data") {
			err := req.ParseMultipartForm(mage.Config.MaxFileUploadSize)
			if err != nil {
				return ctx, err
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
				return ctx, err
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

	return context.WithValue(ctx, KeyRequestInputs, reqValues), nil
}
