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

type specialController struct {
	controllerTest
}

func (c *specialController) Process(ctx context.Context, out *ResponseOutput) Redirect {
	renderer := TextRenderer{}
	renderer.Data = "I AM SPECIAL!"
	out.Renderer = &renderer
	return Redirect{Status:http.StatusOK}
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
	router.SetRoute("/parent/snasi", func() Controller { return &controllerTest{}})
	router.SetRoute("/parent/fnari", func() Controller { return &controllerTest{}})
	router.SetRoute("/parent/:param", func() Controller { return &controllerTest{}})
	router.SetRoute("/parent/*", func() Controller { return &controllerTest{}})
	router.SetRoute("/parent/*/snasi", func() Controller { return &specialController{}})

	m.Config.Router = &router

	app := &appTest{}

	m.LaunchApp(app)

	req, err := instance.NewRequest(http.MethodGet, "/parent/snasi", nil)
	if err != nil {
		t.Fatalf("Error creating request %v", err)
	}
	recorder := httptest.NewRecorder()
	ctx := appengine.NewContext(req)

	printRoutes(ctx, router.root.children, "")

	m.Run(recorder, req)

	if recorder.Code >= http.StatusBadRequest {
		t.Fatalf("Received status %d with body %s", recorder.Code, string(recorder.Body.Bytes()))
	}

	t.Logf("Recorder status %d. Body is %s", recorder.Code, string(recorder.Body.Bytes()))

}