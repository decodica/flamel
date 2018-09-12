package mage

import (
	"fmt"
	"golang.org/x/net/context"
	"google.golang.org/appengine"
	"google.golang.org/appengine/aetest"
	"google.golang.org/appengine/log"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

type appTest struct {
	Application
}

func (app *appTest) OnStart(ctx context.Context) context.Context {
	return ctx
}

//called after each response has been finalized
func (app *appTest) AfterResponse(ctx context.Context) {
}

func (app *appTest) AuthenticatorForPath(path string) Authenticator {
	return nil
}

//controller
type controllerTest struct {}

func (controller *controllerTest) Process(ctx context.Context, out *ResponseOutput) Redirect {
	return Redirect{Status:http.StatusOK}
}

func (controller *controllerTest) OnDestroy(ctx context.Context) {

}

func printRoutes(ctx context.Context, routes map[string]route, parent string) {
	for _, v := range routes {
		var controller Controller = nil
		if v.factory != nil {
			controller = v.factory()
		}
		path := fmt.Sprintf("%s/%s -> Controller: %s", parent, v.name, reflect.TypeOf(controller))
		log.Infof(ctx, "%s", path)
		printRoutes(ctx, v.children, path)
	}
}

func TestMage_Run(t *testing.T) {

	t.Log("*** TEST STARTED ***")

	opts := aetest.Options{}
	instance, err := aetest.NewInstance(&opts)

	if err != nil {
		t.Fatalf("Error creating instance %v", err)
	}
	defer instance.Close()

	//set up mage
	//set up mage instance
	m := Instance()
	router := NewRouter()
	router.SetRoute("/parente/:child/snasi", func() Controller { return &controllerTest{}})

	m.Config.Router = &router

	app := &appTest{}

	m.LaunchApp(app)

	req, err := instance.NewRequest(http.MethodGet, "/parent/1/snasi", nil)
	if err != nil {
		t.Fatalf("Error creating request %v", err)
	}
	recorder := httptest.NewRecorder()
	ctx := appengine.NewContext(req)

	printRoutes(ctx, router.root.children, "")

	m.Run(recorder, req)

	log.Infof(ctx, "Mage is %+v", m)

	if recorder.Code >= http.StatusBadRequest {
		t.Fatalf("Received status %d", recorder.Code)
	}

	t.Logf("Recorder status %d", recorder.Code)

}