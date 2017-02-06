package model

import (
	"google.golang.org/appengine/datastore"
	"golang.org/x/net/context"
	"reflect"
	"fmt"
	"github.com/pkg/errors"
	"log"
)

type Query struct {
	dq *datastore.Query
	mType reflect.Type
}

func NewQuery(m modelable) *Query {
	model := m.getModel();
	if !model.Registered {
		Register(m);
	}

	q := datastore.NewQuery(model.structName);
	query := Query{
		dq: q,
		mType: reflect.TypeOf(m).Elem(),
	}
	log.Printf("modelable is of type %s", query.mType.Name());
	return &query;
}

func (q *Query) With(field string, value interface{}) *Query {
	q.dq = q.dq.Filter(field, value);
	return q;
}

func Get(ctx context.Context, query *Query) ([]modelable, error) {

	if (query.dq == nil) {
		return nil, errors.New("Invalid query. It's nil");
	}

	query.dq = query.dq.KeysOnly();

	var modelables []modelable;

	log.Printf("modelable is of type %s", query.mType.Name());

	_, e := get(ctx, query, &modelables);

	if e != nil && e != datastore.Done {
		return nil, e;
	}

	//data.Print("DONE RECOVERING ITEMS. FOUND " + strconv.Itoa(rc) + " ITEMS FOR ENTITY " + data.entityName);
	//log.Printf("GET CALLED. Models: %+v", models);
	query = nil;
	return modelables, nil;
}

func get(ctx context.Context, query *Query, modelables *[]modelable) (*datastore.Cursor, error) {

	more := false;
	rc := 0;
	it := query.dq.Run(ctx);
	log.Printf("Datastore query is %+v", query.dq);

	for {

		key, err := it.Next(nil);

		if (err == datastore.Done) {
			break;
		}

		if err != nil {
			query = nil;
			return nil, err;
		}

		more = true;
		//log.Printf("RUNNING QUERY %v FOR MODEL " + data.entityName + " - FOUND ITEM WITH KEY: " + strconv.Itoa(int(key.IntID())), data.query);
		newModelable := reflect.New(query.mType);

		log.Printf("Created modelable %v", newModelable);

		m, ok := newModelable.Interface().(modelable);

		if !ok {
			err = fmt.Errorf("Can't cast struct of type %s to modelable", query.mType.Name());
			query = nil;
			return nil, err
		}

		Register(m);

		model := m.getModel()
		model.key = key;

		err = Read(ctx, m);
		if err != nil {
			query = nil;
			return nil, err;
		}

		*modelables = append(*modelables, m);
		rc++;
	}

	if !more {
		//if there are no more entries to be loaded, break the loop
		return nil, datastore.Done;
	} else {
		//else, if we still have entries, update cursor position
		cursor, e := it.Cursor();

		return &cursor, e;
	}
}

