package mage

import (
	"errors"
	"net/http"
	"time"
	"golang.org/x/net/context"
	"google.golang.org/appengine"
	"google.golang.org/appengine/blobstore"
	"google.golang.org/appengine/file"
	"google.golang.org/appengine/image"
	"google.golang.org/appengine/memcache"
	"distudio.com/mage/cors"
	"strings"
	"io/ioutil"
	"fmt"
)

type mage struct {
	//user factory
	Config         MageConfig
	app            Application
}

type Authenticator interface {
	Authenticate(ctx context.Context, user Authenticable) context.Context
}

type Authenticable interface {
	Authenticate(ctx context.Context, token string) error
}

type Application interface {
	NewUser(ctx context.Context) Authenticable
	OnCreate(ctx context.Context) context.Context
	CreatePage(ctx context.Context, path string) (error, Page)
	OnDestroy(ctx context.Context)
	AuthenticatorForPath(path string) Authenticator
}

func (mage *mage) LaunchApp(application Application) {
	if mage.app != nil {
		panic("Application already set")
	}
	mage.app = application
}

type MageConfig struct {
	UseMemcache            bool
	TokenAuthenticationKey string
	TokenExpirationKey     string
	TokenExpiration        time.Duration
	RequestUrlKey          string
	MaxFileUploadSize      int64
	//true if the server suport Cross Origin Request
	CORS *cors.Cors

	//used to enforce redirect to base host url
	EnforceHostnameRedirect string
}

func DefaultConfig() MageConfig {
	config := MageConfig{}
	//set default keys
	config.TokenAuthenticationKey = token_auth_key
	config.TokenExpirationKey = token_expiry_key
	config.TokenExpiration = 60 * time.Minute
	config.MaxFileUploadSize = DEFAULT_MAX_FILE_SIZE;

	return config;
}

const (
	base_name        string = "base"
	token_auth_key   string = "SSID"
	token_expiry_key string = "SSID_EXP"

	//request related special vars
	REQUEST_USER = "mage-user";
	REQUEST_INPUTS = "request-inputs";
	REQUEST_METHOD = "method";
	REQUEST_IPV4 = "remote-address";
	REQUEST_JSON_DATA = "__json__"

	//default at 4 megs
	DEFAULT_MAX_FILE_SIZE = (1 << 20) * 4;

	_error_parse_request string = "Error parsing request data: %v";
	_error_create_page string = "Error creating page: %v";
)

//mage is a singleton
var mageInstance *mage

var bucketName string

func GetBucketName(c context.Context) (string, error) {
	if appengine.IsDevAppServer() {
		appid := appengine.AppID(c)
		return "staging." + appid + ".appspot.com", nil;
	}
	var err error
	if bucketName == "" {
		bucketName, err = file.DefaultBucketName(c)
	}
	return bucketName, err
}

//for now, cache the blobkey in memcache as key/value pair.
//TODO: create a corrispondency "table" with imageName/key in upload/update

func ImageServingUrlString(c context.Context, fileName string) (string, error) {

	if fileName == "" {
		return "", errors.New("Filename must not be empty")
	}

	bucket, err := GetBucketName(c)
	if err != nil {
		return "", err
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
func MageInstance() *mage {

	if mageInstance != nil {
		return mageInstance
	}

	config := DefaultConfig();

	mageInstance = &mage{Config: config}
	return mageInstance
}

func (mage *mage) Run(w http.ResponseWriter, req *http.Request) {

	if mage.app == nil {
		panic("Must set MAGE Application!")
	}

	ctx := appengine.NewContext(req)

	ctx = mage.app.OnCreate(ctx);

	//if we enforce the hostname and the request hostname doesn't match, we redirect to the requested host
	//host is in the form domainname.com
	if mage.Config.EnforceHostnameRedirect != "" && mage.Config.EnforceHostnameRedirect != req.Host {
		scheme := "http://";
		if req.URL.Scheme == "https" {
			scheme = "https://";
		}

		hst := fmt.Sprintf("%s%s%s", scheme, mage.Config.EnforceHostnameRedirect, req.URL.Path);

		if req.URL.RawQuery != "" {
			hst = fmt.Sprintf("%s?%s", hst, req.URL.RawQuery);
		}

		http.Redirect(w, req, hst, http.StatusMovedPermanently);
		renderer := TextRenderer{};
		renderer.Render(w);
		return;
	}

	origin, hasOrigin := req.Header["Origin"];

	//handle CORS requests
	if hasOrigin && mage.Config.CORS != nil && req.Method == http.MethodOptions {
		mage.Config.CORS.HandleOptions(w, origin[0]);
		w.Header().Set("Content-Type", "text/html; charset=utf8");
		renderer := TextRenderer{};
		renderer.Render(w);
		return;
	}

	err, page := mage.app.CreatePage(ctx, req.URL.Path);

	if err != nil {
		renderer := TextRenderer{};
		renderer.Data = fmt.Sprintf(_error_create_page, err);
		w.WriteHeader(http.StatusInternalServerError);
		renderer.Render(w);
		return;
	}

	magePage := newGaemsPage(page);
	defer mage.destroy(ctx, magePage);

	//add inputs to the context object
	ctx, err = mage.parseRequestInputs(ctx, req);
	if err != nil {
		renderer := TextRenderer{};
		renderer.Data = fmt.Sprintf(_error_parse_request, err);
		w.WriteHeader(http.StatusInternalServerError);
		renderer.Render(w);
		return;
	}

	authenticator := mage.app.AuthenticatorForPath(req.URL.Path);

	if authenticator != nil {
		user := mage.app.NewUser(ctx);
		ctx = authenticator.Authenticate(ctx, user);
	}

	//add headers and cookies
	for _, v := range magePage.out.cookies {
		http.SetCookie(w, v)
	}

	//handle the CORS framework
	if mage.Config.CORS != nil {

		//handle the AMP case
		if mage.Config.CORS.AMPForUrl(req.URL.Path) {
			AMPsource, hasSource := req.URL.Query()[cors.AMP_SOURCE_ORIGIN_KEY];

			//if the source is not set the AMP request is invalid
			if !hasSource || len(AMPsource) == 0 {
				w.WriteHeader(http.StatusNotAcceptable);
				return;
			}

			//if the source is set we check if the AMP-Same-Origin is set
			hasSameOrigin := req.Header.Get(cors.AMP_SAME_ORIGIN_HEADER_KEY) == "true";

			if !(hasOrigin || hasSameOrigin) {
				w.WriteHeader(http.StatusNotAcceptable);
				return;
			}

			//if the value of AMP_SAME_ORIGIN is different from true we validate the origin
			//amongst those accepted
			if mage.Config.CORS.ValidateAMP(w, AMPsource[0]) != nil {
				w.WriteHeader(http.StatusNotAcceptable);
				return;
			}
		}

		if hasOrigin {
			allowed := mage.Config.CORS.HandleOptions(w, origin[0]);
			if !allowed {
				w.WriteHeader(http.StatusForbidden);
				return;
			}
		}
	}

	redirect := magePage.process(ctx);

	//add the redirect header if needed
	for k, v := range magePage.out.headers {
		w.Header().Set(k, v)
	}

	if redirect.Status >= 300 && redirect.Status < 400 {
		http.Redirect(w, req, redirect.Location, redirect.Status)
	}

	if redirect.Status >= 400 {
		w.WriteHeader(redirect.Status);
	}

	defer magePage.out.Renderer.Render(w);
}

func (mage *mage) destroy(ctx context.Context, page *magePage) {
	page.page.OnDestroy(ctx);
	mage.app.OnDestroy(ctx);
}

func (mage mage) parseRequestInputs(ctx context.Context, req *http.Request) (context.Context, error) {
	reqValues := make(RequestInputs);

	//todo: put in context
	method := requestInput{};
	m := make([]string, 1, 1);
	m[0] = req.Method;
	method.values = m;
	reqValues[REQUEST_METHOD] = method;


	ipV := requestInput{};
	ip := make([]string, 0);
	ip = append(ip, req.RemoteAddr);
	ipV.values = ip;
	reqValues[REQUEST_IPV4] = ipV;

	//get request params
	switch req.Method {
	case http.MethodGet:
		for k, _ := range req.URL.Query() {
			s := make([]string, 1, 1);
			v := req.URL.Query().Get(k);
			s[0] = v;
			i := requestInput{};
			i.values = s;
			reqValues[k] = i;
		}
		break;
	case http.MethodPut:
		fallthrough
	case http.MethodPost:
		reqType := req.Header.Get("Content-Type");
		//parse the multiform data if the request specifies it as its content type
		if strings.Contains(reqType, "multipart/form-data") {
			err := req.ParseMultipartForm(mage.Config.MaxFileUploadSize);
			if err != nil {
				return ctx, err;
			}
		} else {
			err := req.ParseForm();
			if err != nil {
				return ctx, err;
			}
		}

		if  strings.Contains(reqType, "application/json") {
			s := make([]string, 1, 1);
			i := requestInput{};
			str, _ := ioutil.ReadAll(req.Body);
			s[0] = string(str);
			i.values = s;
			reqValues[REQUEST_JSON_DATA] = i;
			break;
		}

		for k , _ := range req.Form {
			v := req.Form[k];
			i := requestInput{};
			i.values = v;
			reqValues[k] = i;
		}
	case http.MethodDelete:
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

	return context.WithValue(ctx, REQUEST_INPUTS, reqValues), nil;
}
