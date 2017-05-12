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
	//true if the server suport Cross Origin Request
	CORS *cors.Cors
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

	config := MageConfig{}
	//set default keys
	config.TokenAuthenticationKey = token_auth_key
	config.TokenExpirationKey = token_expiry_key
	config.TokenExpiration = 60 * time.Minute

	mageInstance = &mage{Config: config}
	return mageInstance
}

func (mage *mage) Run(w http.ResponseWriter, req *http.Request) {

	if mage.app == nil {
		panic("Must set MAGE Application!")
	}

	ctx := appengine.NewContext(req)

	ctx = mage.app.OnCreate(ctx);

	//handle CORS requests
	if mage.Config.CORS != nil && req.Method == http.MethodOptions {
		w = mage.Config.CORS.HandleOptions(w);
		w.Header().Set("Content-Type", "text/html; charset=utf8");
		renderer := TextRenderer{};
		renderer.Render(w);
		return;
	}

	err, page := mage.app.CreatePage(ctx, req.URL.Path);

	if nil != err {
		panic(err);
	}

	magePage := newGaemsPage(page);
	defer mage.destroy(ctx, magePage);

	//add inputs to the context object
	ctx = magePage.build(ctx, req);


	authenticator := mage.app.AuthenticatorForPath(req.URL.Path);

	if authenticator != nil {
		user := mage.app.NewUser(ctx);
		ctx = authenticator.Authenticate(ctx, user);
	}

	redirect := magePage.process(ctx, req);

	//add headers and cookies
	for _, v := range magePage.out.cookies {
		http.SetCookie(w, v)
	}

	if mage.Config.CORS != nil {
		w = mage.Config.CORS.HandleOptions(w);
	}

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

	magePage.out.Renderer.Render(w);
}

func (mage *mage) destroy(ctx context.Context, page *magePage) {
	page.page.OnDestroy(ctx);
	mage.app.OnDestroy(ctx);
}
