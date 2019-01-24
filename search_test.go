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
	Age  int    `model:"search"`
	Job  Job    `model:"search"`
}

type Job struct {
	model.Model
	Name string
}

//controller
type populateController struct {
	t *testing.T
}

var count = 0

func (controller *populateController) Process(ctx context.Context, out *ResponseOutput) Redirect {

	rigattiere := Job{Name: "Rigattiere"}
	spazzino := Job{Name: "Spazzino"}

	model.Create(ctx, &rigattiere)
	model.Create(ctx, &spazzino)

	for i := 0; i < iterations; i++ {
		m := TestModel{}
		idx := rand.Intn(len(names))
		m.Name = names[idx]
		m.Age = rand.Intn(70) + 18

		if m.Name == "Enzo" {
			m.Job = rigattiere
		} else {
			m.Job = spazzino
		}

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

func (controller *populateController) OnDestroy(ctx context.Context) {

}

// search controller
type searchController struct {
	t *testing.T
}

func (controller *searchController) Process(ctx context.Context, out *ResponseOutput) Redirect {

	sq := model.NewSearchQuery((*TestModel)(nil))
	sq.SearchWith("Name = Enzo")

	results := make([]*TestModel, 0, 0)

	rc, err := sq.Search(ctx, &results, nil)

	if err != nil {
		controller.t.Fatalf("error searching Enzos: %v", err)
	}

	if len(results) != count {
		controller.t.Fatalf("created %d Enzos, but we found %d by name", count, rc)
	}

	for _, enzo := range results {
		if enzo.Job.Name != "Rigattiere" {
			controller.t.Fatalf("enzo has an invalid job: %s", enzo.Job.Name)
		}
	}

	// now we search by jobs

	results = make([]*TestModel, 0, 0)
	rigattiere := Job{}
	query := model.NewQuery(&rigattiere)
	query.WithField("Name =", "Rigattiere")
	err = query.First(ctx, &rigattiere)

	if err != nil {
		controller.t.Fatalf("error retrieving rigattiere: %s", err.Error())
	}

	sq = model.NewSearchQuery((*TestModel)(nil))
	sq.SearchWithModel("Job =", &rigattiere, model.SearchNoOp)
	rc, err = sq.Search(ctx, &results, nil)

	if err != nil {
		controller.t.Fatalf("error retrieving Enzos by job: %v", err)
	}

	if rc != count {
		controller.t.Fatalf("created %d Enzos, but we found %d enzos by job", count, rc)
	}

	for _, enzo := range results {
		if enzo.Job.Name != "Rigattiere" {
			controller.t.Fatalf("enzo has an invalid job: %s", enzo.Job.Name)
		}
	}

	return Redirect{Status: http.StatusOK}
}

func (controller *searchController) OnDestroy(ctx context.Context) {

}

func TestSearch_Run(t *testing.T) {

	t.Log("*** TEST STARTED ***")

	opts := aetest.Options{}
	instance, err := aetest.NewInstance(&opts)

	if err != nil {
		t.Fatalf("Error creating instance %v", err)
	}
	defer instance.Close()

	//set up mage
	m := Instance()
	m.SetRoute("/create", func(ctx context.Context) Controller { return &populateController{t: t} }, nil)
	m.SetRoute("/search", func(ctx context.Context) Controller { return &searchController{t: t} }, nil)

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
