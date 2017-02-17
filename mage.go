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
	applicationSet bool
}

type Authenticable interface {
	Authenticate(ctx context.Context, token string) error
}

type Application interface {
	NewUser(ctx context.Context) Authenticable
	OnCreate(ctx context.Context)
	CreatePage(ctx context.Context, path string) (error, Page)
	OnDestroy(ctx context.Context)
}

func (this *mage) LaunchApp(application Application) {
	if this.applicationSet {
		panic("Application already set")
	}
	this.app = application
	this.applicationSet = true
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

	ctx := appengine.NewContext(req)

	if !mage.applicationSet {
		panic("Must set MAGE Application!")
	}

	mage.app.OnCreate(ctx);
	defer mage.destroy(ctx);

	//todo: should we use the app specific context?
	err, page := mage.app.CreatePage(ctx, req.URL.Path);

	if nil != err {
		//todo: send a 500
		panic(err);
	}

	user := mage.app.NewUser(ctx);

	magePage := newGaemsPage(page, user);
	defer magePage.finish();

	magePage.build(req)

	//populate the user model
	//todo: find a more elegant solution.
	//using the cookie object prevents client application to use
	//non-cookie tokens (es. json-rpc with x-sign in header pattern

	tkn, _ := req.Cookie(MageInstance().Config.TokenAuthenticationKey)

	if tkn != nil {
		err := magePage.currentUser.Authenticate(ctx, tkn.Value)
		if err != nil {
			//todo hanle with a 500
			panic(err);
		}
	}

	redirect := magePage.page.Authorize()

	if redirect.Status != http.StatusOK {
		http.Redirect(w, req, redirect.Location, redirect.Status)
		return
	}

	magePage.process(req)

	//add headers and cookies
	for _, v := range magePage.out.cookies {
		http.SetCookie(w, v)
	}

	//add the redirect header if needed
	for k, v := range magePage.out.headers {
		w.Header().Set(k, v)
	}

	redirect = magePage.out.Redirect
	if redirect.Status != http.StatusOK {
		w.Header().Set("Location", redirect.Location)
		w.WriteHeader(redirect.Status)
	}

	magePage.output(w);
}

func (mage *mage) destroy(ctx context.Context) {
	mage.app.OnDestroy(ctx);
}
