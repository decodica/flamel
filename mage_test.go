package mage

import (
	"golang.org/x/net/context"
	"google.golang.org/appengine/aetest"
	"log"
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

//called after each response has been finalized
func (app *appTest) AfterResponse(ctx context.Context) {
}

func (app *appTest) AuthenticatorForPath(path string) Authenticator {
	return nil
}

//controller
type controllerTest struct {}

func (controller *controllerTest) Process(ctx context.Context, out *ResponseOutput) Redirect {
	in := InputsFromContext(ctx)
	for k, _ := range in {
		log.Printf("key %s -> %s\n", k, in[k].Value())
	}
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
	router.SetRoute("/static", func() Controller { return &controllerTest{}})
	router.SetRoute("/parent/*/wildcard", func() Controller { return &controllerTest{}})
	router.SetRoute("/parent/:id", func() Controller { return &controllerTest{}})
	router.SetRoute("/parent/:id/:children", func() Controller { return &controllerTest{}})
	router.SetRoute("/parent", func() Controller { return &specialController{}})

	log.Printf("----- TREE WALKING START -----\n")
	recursiveWalk(router.tree.root, "/parent/:id/:children")
	log.Printf("----- TREE WALKING END -----\n")

	log.Printf("----- TREE WALKING START -----\n")
	recursiveWalk(router.tree.root, "/parent/:id")
	log.Printf("----- TREE WALKING END -----\n")

	m.Config.Router = &router

	app := &appTest{}

	m.LaunchApp(app)

	req, err := instance.NewRequest(http.MethodGet, "/parent/1/2", nil)
	if err != nil {
		t.Fatalf("Error creating request %v", err)
	}
	recorder := httptest.NewRecorder()
	m.Run(recorder, req)

	if recorder.Code >= http.StatusBadRequest {
		t.Fatalf("Received status %d with body %s", recorder.Code, string(recorder.Body.Bytes()))
	}

	t.Logf("Recorder status %d. Body is %s", recorder.Code, string(recorder.Body.Bytes()))

}