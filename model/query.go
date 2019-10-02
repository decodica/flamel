package model

import (
	"cloud.google.com/go/datastore"
	"errors"
	"fmt"
	"golang.org/x/net/context"
	"google.golang.org/api/iterator"
	"reflect"
)

type Query struct {
	dq         *datastore.Query
	mType      reflect.Type
	projection bool
}

type Order uint8

const (
	ASC Order = iota + 1
	DESC
)

func NewQuery(m modelable) *Query {
	typ := reflect.TypeOf(m).Elem()

	q := datastore.NewQuery(typ.Name())
	query := Query{
		dq:         q,
		mType:      typ,
		projection: false,
	}
	return &query
}

/**
Filter functions
*/
func (q *Query) WithModelable(field string, ref modelable) *Query {
	refm := ref.getModel()
	if !refm.registered {
		panic(fmt.Errorf("modelable reference is not registered %+v", ref))
	}

	if refm.Key == nil {
		panic(errors.New("reference Key has not been set. Can't retrieve it from datastore"))
	}

	if _, ok := q.mType.FieldByName(field); !ok {
		panic(fmt.Errorf("struct of type %s has no field with name %s", q.mType.Name(), field))
	}

	return q.WithField(fmt.Sprintf("%s = ", field), refm.Key)
}

func (q *Query) WithAncestor(ancestor modelable) (*Query, error) {
	am := ancestor.getModel()
	if am.Key == nil {
		return nil, fmt.Errorf("invalid ancestor. %s has empty Key", am.Name())
	}

	q.dq = q.dq.Ancestor(am.Key)
	return q, nil
}

func (q *Query) WithField(field string, value interface{}) *Query {
	prepared := field
	q.dq = q.dq.Filter(prepared, value)
	return q
}

func (q *Query) OrderBy(field string, order Order) *Query {
	prepared := field
	if order == DESC {
		prepared = fmt.Sprintf("-%s", prepared)
	}
	q.dq = q.dq.Order(prepared)
	return q
}

func (q *Query) OffsetBy(offset int) *Query {
	q.dq = q.dq.Offset(offset)
	return q
}

func (q *Query) Limit(limit int) *Query {
	q.dq = q.dq.Limit(limit)
	return q
}

func (q *Query) Count(ctx context.Context) (int, error) {
	client := ClientFromContext(ctx)
	return client.Count(ctx, q.dq)
}

func (q *Query) Distinct(fields ...string) *Query {
	q.dq = q.dq.Project(fields...)
	q.dq = q.dq.Distinct()
	q.projection = true
	return q
}

func (q *Query) Project(fields ...string) *Query {
	q.dq = q.dq.Project(fields...)
	q.projection = true
	return q
}

//Shorthand method to retrieve only the first entity satisfying the query
//It is equivalent to a Get With limit 1
func (q *Query) First(ctx context.Context, m modelable) (err error) {
	q.dq = q.dq.Limit(1)

	var mm []modelable

	err = q.GetAll(ctx, &mm)

	if err != nil {
		return err
	}

	if len(mm) > 0 {
		src := reflect.Indirect(reflect.ValueOf(mm[0]))
		reflect.Indirect(reflect.ValueOf(m)).Set(src)
		index(m)
		return nil
	}

	return datastore.ErrNoSuchEntity
}

func (query *Query) Get(ctx context.Context, dst interface{}) error {
	if query.dq == nil {
		return errors.New("invalid query. Query is nil")
	}

	defer func() {
		query = nil
	}()

	if !query.projection {
		query.dq = query.dq.KeysOnly()
	}

	_, err := query.get(ctx, dst)

	if err != nil && err != iterator.Done {
		return err
	}

	return nil
}

func (query *Query) GetAll(ctx context.Context, dst interface{}) error {
	if query.dq == nil {
		return errors.New("invalid query. Query is nil")
	}

	defer func() {
		query = nil
	}()

	if !query.projection {
		query.dq = query.dq.KeysOnly()
	}

	var cursor *datastore.Cursor
	var e error

	done := false

	for !done {

		if cursor != nil {
			query.dq = query.dq.Start(*cursor)
		}

		cursor, e = query.get(ctx, dst)

		if e != iterator.Done && e != nil {
			return e
		}

		done = e == iterator.Done
	}

	return nil
}

func (query *Query) GetMulti(ctx context.Context, dst interface{}) error {
	if query.dq == nil {
		return errors.New("invalid query. Query is nil")
	}

	defer func() {
		query = nil
	}()

	if query.projection {
		return errors.New("invalid query. Can't use projection queries with GetMulti")
	}

	client := ClientFromContext(ctx)
	query.dq = query.dq.KeysOnly()
	it := client.Run(ctx, query.dq)

	dstv := reflect.ValueOf(dst)

	if !isValidContainer(dstv) {
		return fmt.Errorf("invalid container of type %s. Container must be a modelable slice", dstv.Elem().Type().Name())
	}

	modelables := dstv.Elem()

	for {
		key, err := it.Next(nil)

		if err == iterator.Done {
			break
		}

		if err != nil {
			return err
		}

		newModelable := reflect.New(query.mType)
		m, ok := newModelable.Interface().(modelable)

		if !ok {
			err = fmt.Errorf("can't cast struct of type %s to modelable", query.mType.Name())
			query = nil
			return err
		}

		//Note: indexing here assigns the address of m to the Model.
		//this means that if a user supplied a populated dst we must reindex its elements before returning
		//or the model will point to a different modelable
		index(m)

		model := m.getModel()
		model.Key = key

		modelables.Set(reflect.Append(modelables, reflect.ValueOf(m)))
	}

	return ReadMulti(ctx, reflect.Indirect(dstv).Interface())
}

func (query *Query) get(ctx context.Context, dst interface{}) (*datastore.Cursor, error) {

	client := ClientFromContext(ctx)

	more := false
	rc := 0

	it := client.Run(ctx, query.dq)

	dstv := reflect.ValueOf(dst)

	if !isValidContainer(dstv) {
		return nil, fmt.Errorf("invalid container of type %s. Container must be a modelable slice", dstv.Elem().Type().Name())
	}

	modelables := dstv.Elem()

	for {

		Key, err := it.Next(nil)

		if err == iterator.Done {
			break
		}

		if err != nil {
			query = nil
			return nil, err
		}

		more = true
		//log.Printf("RUNNING QUERY %v FOR MODEL " + data.entityName + " - FOUND ITEM WITH KEY: " + strconv.Itoa(int(Key.IntID())), data.query);
		newModelable := reflect.New(query.mType)
		m, ok := newModelable.Interface().(modelable)

		if !ok {
			err = fmt.Errorf("can't cast struct of type %s to modelable", query.mType.Name())
			query = nil
			return nil, err
		}

		//todo Note: indexing here assigns the address of m to the Model.
		//this means that if a user supplied a populated dst we must reindex its elements before returning
		//or the model will point to a different modelable
		index(m)

		model := m.getModel()
		model.Key = Key

		err = Read(ctx, m)
		if err != nil {
			query = nil
			return nil, err
		}
		modelables.Set(reflect.Append(modelables, reflect.ValueOf(m)))
		rc++
	}

	if !more {
		//if there are no more entries to be loaded, break the loop
		return nil, iterator.Done
	} else {
		//else, if we still have entries, update cursor position
		cursor, e := it.Cursor()
		return &cursor, e
	}
}

//container must be *[]modelable
func isValidContainer(container reflect.Value) bool {
	if container.Kind() != reflect.Ptr {
		return false
	}
	celv := container.Elem()
	if celv.Kind() != reflect.Slice {
		return false
	}

	cel := celv.Type().Elem()
	ok := cel.Implements(typeOfModelable)
	if !ok {
	}
	return ok
}
