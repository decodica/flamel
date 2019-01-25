package mage

import (
	"context"
	"distudio.com/mage/model"
	"fmt"
	"google.golang.org/appengine/aetest"
	"google.golang.org/appengine/memcache"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const name = "EntityName"
const age int = 50

type TestEntity struct {
	model.Model
	Name     string
	Child    TestChild
	Nocreate TestNocreate
}

type TestChild struct {
	model.Model
	Name string
}

type TestNocreate struct {
	model.Model
	Num int
}

type testController struct {
	t *testing.T
}

func (controller *testController) OnDestroy(ctx context.Context) {}

// creates the testentity
type createController struct {
	*testController
}

func (controller *createController) Process(ctx context.Context, out *ResponseOutput) Redirect {
	entity := TestEntity{}
	empty := model.IsEmpty(&entity)
	if !empty {
		msg := fmt.Sprintf("entity %+v should be empty", entity)
		controller.t.Log(msg)
		return Redirect{Status: http.StatusConflict}
	}

	entity.Name = name
	entity.Child.Name = "ChildName"

	empty = model.IsEmpty(&entity)
	if empty {
		msg := fmt.Sprintf("entity %+v should not be empty", entity)
		controller.t.Log(msg)
		return Redirect{Status: http.StatusConflict}
	}

	err := model.Create(ctx, &entity)

	if err != nil {
		msg := fmt.Sprintf("error creating entity: %s", err.Error())
		controller.t.Log(msg)
		return Redirect{Status: http.StatusInternalServerError}
	}

	empty = model.IsEmpty(&entity)
	if empty {
		msg := fmt.Sprintf("entity %+v should not be empty", entity)
		controller.t.Log(msg)
		return Redirect{Status: http.StatusConflict}
	}

	err = memcache.Flush(ctx)
	if err != nil {
		return Redirect{Status: http.StatusInternalServerError}
	}
	return Redirect{Status: http.StatusOK}
}

// updates the testentity
type updateController struct {
	*testController
}

func (controller *updateController) Process(ctx context.Context, out *ResponseOutput) Redirect {
	te := TestEntity{}
	q := model.NewQuery(&te)
	q = q.WithField("Name =", name)
	err := q.First(ctx, &te)

	if err != nil {
		msg := fmt.Sprintf("error retrieving entity with name = %s: %s", name, err.Error())
		controller.t.Log(msg)
		return Redirect{Status: http.StatusInternalServerError}
	}

	te.Child.Name = "UpdatedName"
	te.Nocreate.Num = age

	err = model.Update(ctx, &te)
	if err != nil {
		msg := fmt.Sprintf("error updating entity: %s", err.Error())
		controller.t.Log(msg)
		return Redirect{Status: http.StatusInternalServerError}
	}

	err = memcache.Flush(ctx)
	if err != nil {
		return Redirect{Status: http.StatusInternalServerError}
	}

	return Redirect{Status: http.StatusOK}
}

// reads the test entity
type readController struct {
	*testController
}

func (controller *readController) Process(ctx context.Context, out *ResponseOutput) Redirect {
	ins := RoutingParams(ctx)
	arg, ok := ins["type"]
	if !ok {
		msg := fmt.Sprintf("wrong inputs: %+v", ins)
		controller.t.Log(msg)
		return Redirect{Status: http.StatusBadRequest}
	}

	switch arg.Value() {
	case "name":
		te := TestEntity{}
		q := model.NewQuery(&te)
		q = q.WithField("Name = ", name)
		err := q.First(ctx, &te)
		if err != nil {
			msg := fmt.Sprintf("error creating testEntity: %s", err)
			controller.t.Log(msg)
			return Redirect{Status: http.StatusInternalServerError}
		}

		if te.Name != name {
			msg := fmt.Sprintf("entity name is different from %s", name)
			controller.t.Log(msg)
			return Redirect{Status: http.StatusExpectationFailed}
		}

		if !model.IsEmpty(&te.Nocreate) {
			msg := fmt.Sprintf("nocreate must be empty")
			controller.t.Log(msg)
			return Redirect{Status: http.StatusExpectationFailed}
		}
	// call this to check if the update was successful
	case "age":
		te := TestEntity{}
		q := model.NewQuery(&te)
		q = q.WithField("Name = ", name)
		err := q.First(ctx, &te)
		if err != nil {
			msg := fmt.Sprintf("error creating testEntity: %s", err)
			controller.t.Log(msg)
			return Redirect{Status: http.StatusInternalServerError}
		}

		if te.Nocreate.Num != age {
			msg := fmt.Sprintf("nocreate num is different from %s", name)
			controller.t.Log(msg)
			return Redirect{Status: http.StatusExpectationFailed}
		}
	}

	err := memcache.Flush(ctx)
	if err != nil {
		return Redirect{Status: http.StatusInternalServerError}
	}

	return Redirect{Status: http.StatusOK}
}

func TestModelCalls_Run(t *testing.T) {

	t.Log("*** TEST STARTED ***")
	opts := aetest.Options{}
	instance, err := aetest.NewInstance(&opts)
	if err != nil {
		t.Fatalf("error creating ae instance: %s", err.Error())
	}
	defer instance.Close()

	m := Instance()

	app := &appTest{}
	m.LaunchApp(app)

	m.SetRoute("/create", func(ctx context.Context) Controller {
		tc := testController{t: t}
		return &createController{testController: &tc}
	}, nil)

	m.SetRoute("/update", func(ctx context.Context) Controller {
		tc := testController{t: t}
		return &updateController{testController: &tc}
	}, nil)

	m.SetRoute("/read/:type", func(ctx context.Context) Controller {
		params := RoutingParams(ctx)
		for k, v := range params {
			msg := fmt.Sprintf("param %s -> %v", k, v)
			t.Log(msg)
		}
		tc := testController{t: t}
		return &readController{testController: &tc}
	}, nil)

	tryRequest(t, instance, "/create")
	// sleep before making the second request so datastore syncronizes
	time.Sleep(3 * time.Second)

	// read the datastore and verify consistency
	tryRequest(t, instance, "/read/name")
	time.Sleep(3 * time.Second)

	// update the datastore
	tryRequest(t, instance, "/update")
	time.Sleep(3 * time.Second)

	tryRequest(t, instance, "/read/age")
	time.Sleep(3 * time.Second)
}

func tryRequest(t *testing.T, instance aetest.Instance, endpoint string) {
	req, err := instance.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("error creating %q request: %s", endpoint, err.Error())
	}

	recorder := httptest.NewRecorder()
	Instance().Run(recorder, req)

	if recorder.Code >= http.StatusBadRequest {
		t.Fatalf("received bad status %d. Body is %q", recorder.Code, string(recorder.Body.Bytes()))
	}

	t.Logf("'read request: %s", string(recorder.Body.Bytes()))
	recorder.Flush()
}
