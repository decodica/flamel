package mage

import (
	"golang.org/x/net/context"
	"google.golang.org/appengine"
	"google.golang.org/appengine/aetest"
	"google.golang.org/appengine/log"
	"net/http"
	"net/http/httptest"
	"testing"
)

type appTest struct {
	Application
}

func (app *appTest) OnStart(ctx context.Context) context.Context {
	return ctx
}

func (app *appTest) ControllerForPath(ctx context.Context, path string) (error, Controller) {
	return nil, &controllerTest{}
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

func TestMage_Run(t *testing.T) {

	t.Log("*** TEST STARTED ***")

	opts := aetest.Options{}
	instance, err := aetest.NewInstance(&opts)

	if err != nil {
		t.Fatalf("Error creating instance %v", err)
	}
	defer instance.Close()

	req, err := instance.NewRequest(http.MethodGet, "/", nil)

	if err != nil {
		t.Fatalf("Error creating request %v", err)
	}

	recorder := httptest.NewRecorder()

	m := Instance()
	app := &appTest{}

	m.LaunchApp(app)

	m.Run(recorder, req)

	ctx := appengine.NewContext(req)
	log.Infof(ctx, "Mage is %+v", m)

	if recorder.Code >= http.StatusBadRequest {
		t.Fatalf("Received status %d", recorder.Code)
	}

	t.Logf("Recorder status %d", recorder.Code)

}