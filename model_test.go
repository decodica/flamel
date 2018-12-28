package mage

import (
	"distudio.com/mage/model"
	"golang.org/x/net/context"
	"google.golang.org/appengine/aetest"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var names = []string{
	"Mario",
	"Antonio",
	"Giuseppe",
	"Kevin",
	"Giraudo",
	"Bernardo",
	"Enzo",
	"Gualtiero",
	"Nevio",
	"Ignazio",
}

const iterations = 100

type TestModel struct {
	model.Model
	Name string `model:"search"`
	Age int `model:"search"`
}

//controller
type createController struct {
	t *testing.T
}

func (controller *createController) Process(ctx context.Context, out *ResponseOutput) Redirect {

	count := 0
	for i := 0; i < iterations; i++ {
		m := TestModel{}
		idx := rand.Intn(len(names))
		m.Name = names[idx]
		m.Age = rand.Intn(70) + 18
		err := model.Create(ctx, &m)
		if err != nil {
			controller.t.Fatalf("error creating entities %v", err)
		}

		if m.Name == "Enzo" {
			count++
		}
	}

	controller.t.Logf("Created %d Enzos", count)
	return Redirect{Status: http.StatusOK}
}

func (controller *createController) OnDestroy(ctx context.Context) {

}


// search controller
type searchController struct {
	t *testing.T
}

func (controller *searchController) Process(ctx context.Context, out *ResponseOutput) Redirect {

	sq := model.NewSearchQuery((*TestModel)(nil))
	sq.SearchWith("Name = Enzo")

	results := []*TestModel{}

	err := sq.Search(ctx, &results, nil)

	if err != nil {
		controller.t.Fatalf("error searching Enzos: %v", err)
	}

	controller.t.Logf("found %d Enzos", len(results))

	return Redirect{Status: http.StatusOK}
}

func (controller *searchController) OnDestroy(ctx context.Context) {

}

func TestModel_Run(t *testing.T) {

	t.Log("*** TEST STARTED ***")

	opts := aetest.Options{}
	instance, err := aetest.NewInstance(&opts)

	if err != nil {
		t.Fatalf("Error creating instance %v", err)
	}
	defer instance.Close()

	//set up mage
	m := Instance()
	m.SetRoute("/create", func(ctx context.Context) Controller { return &createController{t:t} }, nil)
	m.SetRoute("/search", func(ctx context.Context) Controller { return &searchController{t:t} }, nil)

	app := &appTest{}

	m.LaunchApp(app)

	req, err := instance.NewRequest(http.MethodGet, "/create", nil)

	if err != nil {
		t.Fatalf("Error creating request %v", err)
	}
	recorder := httptest.NewRecorder()
	m.Run(recorder, req)

	if recorder.Code >= http.StatusBadRequest {
		t.Fatalf("Received status %d with body %s", recorder.Code, string(recorder.Body.Bytes()))
	}

	t.Logf("Recorder status %d. Body is %s", recorder.Code, string(recorder.Body.Bytes()))

	// wait for results to be written, then run searches against the model set
	t.Logf("Sleeping for 3 seconds...")
	time.Sleep(3 * time.Second)
	t.Logf("Done sleeping...")

	// run searches
	req, err = instance.NewRequest(http.MethodGet, "/search", nil)

	if err != nil {
		t.Fatalf("Error creating request %v", err)
	}
	recorder = httptest.NewRecorder()
	m.Run(recorder, req)

	if recorder.Code >= http.StatusBadRequest {
		t.Fatalf("Received status %d with body %s", recorder.Code, string(recorder.Body.Bytes()))
	}

	t.Logf("Recorder status %d. Body is %s", recorder.Code, string(recorder.Body.Bytes()))

}
