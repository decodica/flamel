package mage

import (
	"golang.org/x/net/context"
	"google.golang.org/appengine"
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
type controllerTest struct {
	name string
}

func (controller *controllerTest) Process(ctx context.Context, out *ResponseOutput) Redirect {
	in := InputsFromContext(ctx)
	for k, _ := range in {
		log.Printf("key %s -> %s\n", k, in[k].Value())
	}
	renderer := TextRenderer{}
	renderer.Data = controller.name
	out.Renderer = &renderer
	return Redirect{Status: http.StatusOK}
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

	//set up mage
	//set up mage instance
	m := Instance()
	m.SetRoute("/static", func(ctx context.Context) Controller { return &controllerTest{name: "/static"} })
	//	router.SetRoute("/sta", func() Controller { return &controllerTest{name:"/sta"}})
	//	router.SetRoute("/static/*/wildcard", func() Controller { return &controllerTest{name:"/static/*/wildcard"}})
	m.SetRoute("/static/*", func(ctx context.Context) Controller { return &controllerTest{name: "/static/*"} })
	m.SetRoute("/static/carlo", func(ctx context.Context) Controller { return &controllerTest{name: "/static/carlo"} })
	//	router.SetRoute("/static/:value", func() Controller { return &controllerTest{name:"/static/:value"}})
	m.SetRoute("/param/:param", func(ctx context.Context) Controller { return &controllerTest{name: "/param/:value"} })
	m.SetRoute("/param/:param/end", func(ctx context.Context) Controller { return &controllerTest{name: "/param/:value/end"} })
	m.SetRoute("/param/:param/end/:end", func(ctx context.Context) Controller { return &controllerTest{name: "/param/:value/end/:end"} })
	m.SetRoute("/*", func(ctx context.Context) Controller { return &controllerTest{name: "/*"} })

	app := &appTest{}

	m.LaunchApp(app)

	req, err := instance.NewRequest(http.MethodGet, "/param/3/end", nil)
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

func BenchmarkFindRoute(b *testing.B) {

	opts := aetest.Options{}
	instance, err := aetest.NewInstance(&opts)

	if err != nil {
		b.Fatalf("Error creating instance %v", err)
	}
	defer instance.Close()

	//set up mage
	//set up mage instance
	m := Instance()

	m.SetRoute("/param/:param/end/:end", func(ctx context.Context) Controller { return &controllerTest{name: "/param/:value/end/:end"} })

	app := &appTest{}

	m.LaunchApp(app)

	req, err := instance.NewRequest(http.MethodGet, "/param/5/end/7", nil)
	if err != nil {
		b.Fatalf("Error creating request %v", err)
	}
	ctx := appengine.NewContext(req)

	if err != nil {
		b.Fatalf("Error creating context: %s", err)
	}

	b.Run("Find route", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			err, _ = m.Router.RouteForPath(ctx, req.URL.Path)
			if err != nil {
				b.Fatalf("Error retrieving route: %s", err)
			}
		}

	})

}
