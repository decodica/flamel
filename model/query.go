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
	mValue reflect.Value
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

/**
Filter functions
 */
func (q *Query) WithModelable(field string, ref modelable) (*Query, error) {
	refm := ref.getModel();
	if !refm.Registered {
		return nil, fmt.Errorf("Modelable reference is not registered %+v", ref);
	}

	if refm.key == nil {
		return nil, errors.New("Reference key has not been set. Can't retrieve it from datastore");
	}

	if _, ok := q.mType.FieldByName(field); !ok {
		return nil, fmt.Errorf("Struct of type %s has no field with name %s", q.mType.Name(), field);
	}

	refName := referenceName(q.mType.Name(), field);

	return q.WithField(fmt.Sprintf("%s = ", refName), refm.key), nil;
}

func (q *Query) WithField(field string, value interface{}) *Query {
	q.dq = q.dq.Filter(field, value);
	return q;
}

func (q *Query) OrderBy(fieldName string) *Query {
	q.dq = q.dq.Order(fieldName);
	return q;
}

func (q *Query) OffsetBy(offset int) *Query {
	q.dq = q.dq.Offset(offset);
	return q;
}

func (q *Query) Limit(limit int) *Query {
	q.dq = q.dq.Limit(limit);
	return q;
}

func (q *Query) Count(ctx context.Context) (int, error) {
	return q.dq.Count(ctx);
}

//Shorthand method to retrieve only the first entity satisfying the query
//It is equivalent to a Get With limit 1
func (q *Query) First(ctx context.Context) (modelable, error) {
	q.dq = q.dq.Limit(1);
	res, err := Get(ctx, q);

	log.Printf("Get errors %v", err);
	if err != nil {
		return nil, err;
	}

	if len(res) > 0 {
		return res[0], nil;
	}

	return nil, datastore.ErrNoSuchEntity;
}

func Get(ctx context.Context, query *Query) ([]modelable, error) {

	if (query.dq == nil) {
		return nil, errors.New("Invalid query. It's nil");
	}

	query.dq = query.dq.KeysOnly();

	var modelables []modelable;

	_, e := get(ctx, query, &modelables);

	if e != nil && e != datastore.Done {
		return nil, e;
	}

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

