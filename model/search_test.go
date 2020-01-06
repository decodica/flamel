package model

import (
	"bytes"
	"golang.org/x/net/context"
	"google.golang.org/appengine"
	"google.golang.org/appengine/aetest"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
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

type SearchableModel struct {
	Model
	Name string `model:"search"`
	Age  int    `model:"search"`
	Job  Job    `model:"search"`
}

type Job struct {
	Model
	Name string
}

var count = 0

func resetDatastoreEmulator(t *testing.T) {
	if addr := os.Getenv("DATASTORE_EMULATOR_HOST"); addr != "" {

		var buf bytes.Buffer
		resp, err := http.Post("http://"+addr+"/reset", "application/json", &buf)
		if err != nil {
			t.Logf("unable to reset datastore emulator: %s", err.Error())
			return
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Logf("invalid response: %s", err.Error())
		}

		t.Logf("Datastore emulator: %s", string(body))
	}
}

func newContextWithStartupTime(t *testing.T, secs int) (func(), context.Context) {
	opts := aetest.Options{}
	opts.StartupTimeout = time.Duration(secs) * time.Second
	hasEmu := true
	opts.SupportDatastoreEmulator = &hasEmu
	opts.StronglyConsistentDatastore = true
	inst, err := aetest.NewInstance(&opts)
	if err != nil {
		t.Fatalf("error creating instance: %s", err.Error())
	}

	req, err := inst.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatalf("error creating new request: %s", err.Error())
	}

	return func() {
		if err := inst.Close(); err != nil {
			panic(err)
		}
	}, appengine.NewContext(req)
}

func populateSearch(ctx context.Context, t *testing.T) {

	// reset the datastore emulator
	resetDatastoreEmulator(t)

	rigattiere := Job{Name: "Rigattiere"}
	spazzino := Job{Name: "Spazzino"}

	err := Create(ctx, &rigattiere)
	err = Create(ctx, &spazzino)
	if err != nil {
		t.Fatalf("unable to populate datastore: %s", err.Error())
	}

	for i := 0; i < iterations; i++ {
		m := SearchableModel{}
		idx := rand.Intn(len(names))
		m.Name = names[idx]
		m.Age = rand.Intn(70) + 18

		if m.Name == "Enzo" {
			m.Job = rigattiere
		} else {
			m.Job = spazzino
		}

		err := Create(ctx, &m)
		if err != nil {
			t.Fatalf("error creating entities %v", err)
		}

		if m.Name == "Enzo" {
			count++
		}
	}
	t.Logf("Created %d Enzos", count)
}

// the client libraries require both the datastore emulator and the project id to work.
// thus, set the following environmental variables before running the tests
// DATASTORE_EMULATOR_HOST (the emulator defaults at localhost:8081)
// DATASTORE_PROJECT_ID to any value

func TestSearch(t *testing.T) {

	done, ctx := newContextWithStartupTime(t, 60)
	defer done()

	service := Service{}
	service.Initialize()

	ctx = service.OnStart(ctx)
	defer service.OnEnd(ctx)

	populateSearch(ctx, t)

	sq := NewSearchQuery((*SearchableModel)(nil))
	sq.SearchWith("Name = Enzo")

	results := make([]*SearchableModel, 0, 0)

	rc, err := sq.Search(ctx, &results, nil)

	if err != nil {
		t.Fatalf("error searching Enzos: %v", err)
	}

	// test result count consistency
	if len(results) != count {
		t.Fatalf("created %d Enzos, but we found %d by name", count, rc)
	}

	// test that all enzo's are rigattiere
	for _, enzo := range results {
		if enzo.Job.Name != "Rigattiere" {
			t.Fatalf("enzo has an invalid job: %s", enzo.Job.Name)
		}
	}

	// now we search by jobs
	results = make([]*SearchableModel, 0, 0)
	rigattiere := Job{}
	query := NewQuery(&rigattiere)
	query.WithField("Name =", "Rigattiere")
	err = query.First(ctx, &rigattiere)

	if err != nil {
		t.Fatalf("error retrieving rigattiere: %s", err.Error())
	}

	sq = NewSearchQuery((*SearchableModel)(nil))
	sq.SearchWithModel("Job =", &rigattiere, SearchNoOp)
	rc, err = sq.Search(ctx, &results, nil)

	if err != nil {
		t.Fatalf("error retrieving Enzos by job: %v", err)
	}

	if rc != count {
		t.Fatalf("created %d Enzos, but we found %d enzos by job", count, rc)
	}

	for _, enzo := range results {
		if enzo.Job.Name != "Rigattiere" {
			t.Fatalf("enzo has an invalid job: %s", enzo.Job.Name)
		}
	}
}
