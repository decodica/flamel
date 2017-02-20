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
)

type mage struct {
	//user factory
	Config         MageConfig
	app            Application
}

type Authenticable interface {
	Authenticate(ctx context.Context, token string) error
}

type Application interface {
	NewUser(ctx context.Context) Authenticable
	OnCreate(ctx context.Context) context.Context
	CreatePage(ctx context.Context, path string) (error, Page)
	OnDestroy(ctx context.Context)
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
	err, page := mage.app.CreatePage(ctx, req.URL.Path);

	if nil != err {
		panic(err);
	}

	magePage := newGaemsPage(page);
	defer mage.destroy(ctx, magePage);

	//add inputs to the context object
	ctx = magePage.build(ctx, req);


	//todo: add middleware to care of authentication?
	//using the cookie object prevents client application to use
	//non-cookie tokens (es. json-rpc with x-sign in header pattern
	tkn, _ := req.Cookie(MageInstance().Config.TokenAuthenticationKey)

	if tkn != nil {
		user := mage.app.NewUser(ctx);
		err := user.Authenticate(ctx, tkn.Value)
		if err != nil {
			user = nil;
		}
		//put the user into the context together with the other params
		ctx = context.WithValue(ctx, REQUEST_USER, user);
	}

	redirect := magePage.process(ctx, req);

	if redirect.Status != http.StatusOK {
		http.Redirect(w, req, redirect.Location, redirect.Status)
		return
	}

	//add headers and cookies
	for _, v := range magePage.out.cookies {
		http.SetCookie(w, v)
	}

	//add the redirect header if needed
	for k, v := range magePage.out.headers {
		w.Header().Set(k, v)
	}

	magePage.out.Renderer.Render(w);
}

func (mage *mage) destroy(ctx context.Context, page *magePage) {
	page.page.OnDestroy(ctx);
	mage.app.OnDestroy(ctx);
}
