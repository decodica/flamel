package model

import (
	"google.golang.org/appengine/datastore"
	"golang.org/x/net/context"
	"reflect"
	"fmt"
	"errors"
)

type Query struct {
	dq *datastore.Query
	mType reflect.Type
	projection bool
}

type Order uint8;

const (
	ASC Order = iota + 1
	DESC
)

func NewQuery(m modelable) *Query {
	typ := reflect.TypeOf(m).Elem();

	q := datastore.NewQuery(typ.Name());
	query := Query{
		dq: q,
		mType: typ,
		projection: false,
	}
	return &query;
}

/**
Filter functions
 */
func (q *Query) WithModelable(field string, ref modelable) (*Query, error) {
	refm := ref.getModel();
	if !refm.registered {
		return nil, fmt.Errorf("Modelable reference is not registered %+v", ref);
	}

	if refm.Key == nil {
		return nil, errors.New("Reference Key has not been set. Can't retrieve it from datastore");
	}

	if _, ok := q.mType.FieldByName(field); !ok {
		return nil, fmt.Errorf("Struct of type %s has no field with name %s", q.mType.Name(), field);
	}

	refName := referenceName(q.mType.Name(), field);

	return q.WithField(fmt.Sprintf("%s = ", refName), refm.Key), nil;
}

func (q *Query) WithAncestor(ancestor modelable) (*Query, error) {
	am := ancestor.getModel();
	if am.Key == nil {
		return nil, fmt.Errorf("Invalid ancestor. %s has empty Key", am.Name());
	}

	q.dq = q.dq.Ancestor(am.Key);
	return q, nil;
}

func (q *Query) WithField(field string, value interface{}) *Query {
	prepared := entityPropName(q.mType.Name(), field);
	q.dq = q.dq.Filter(prepared, value);
	return q;
}

func (q *Query) OrderBy(field string, order Order) *Query {
	prepared := entityPropName(q.mType.Name(), field);
	if order == DESC {
		prepared = fmt.Sprintf("-%s", prepared);
	}
	q.dq = q.dq.Order(prepared);
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

func (q *Query) Distinct(fields ...string) * Query {
	pf := make([]string, 0, 0);

	for _ , v := range fields {
		prepared := entityPropName(q.mType.Name(), v);
		pf = append(pf, prepared);
	}

	q.dq = q.dq.Project(pf...);
	q.dq = q.dq.Distinct();
	q.projection = true;
	return q;
}

//Shorthand method to retrieve only the first entity satisfying the query
//It is equivalent to a Get With limit 1
func (q *Query) First(ctx context.Context, m modelable) (err error) {
	q.dq = q.dq.Limit(1);

	mm := []modelable{}

	err = q.GetAll(ctx, &mm);

	if err != nil {
		return err;
	}

	if len(mm) > 0 {
		src := reflect.Indirect(reflect.ValueOf(mm[0]));
		reflect.Indirect(reflect.ValueOf(m)).Set(src);
		index(m);
		return nil;
	}

	return datastore.ErrNoSuchEntity;
}

func (query *Query) Get(ctx context.Context, dst interface{}) error {
	if query.dq == nil {
		return errors.New("Invalid query. Query is nil");
	}

	defer func() {
		query = nil;
	}()

	if !query.projection {
		query.dq = query.dq.KeysOnly();
	}

	_, err := query.get(ctx, dst);


	if err != nil && err != datastore.Done {
		return err;
	}

	return nil;
}

func (query *Query) GetAll(ctx context.Context, dst interface{}) error {
	if query.dq == nil {
		return errors.New("Invalid query. Query is nil");
	}

	defer func() {
		query = nil;
	}()

	if !query.projection {
		query.dq = query.dq.KeysOnly();
	}


	var cursor *datastore.Cursor;
	var e error;

	done := false;

	for !done {

		if cursor != nil {
			query.dq = query.dq.Start(*cursor);
		}

		cursor, e = query.get(ctx, dst);

		if e != datastore.Done && e != nil {
			return e
		}

		done = e == datastore.Done;
	}

	return nil;
}

func (query *Query) GetMulti(ctx context.Context, dst interface{}) error {
	if query.dq == nil {
		return errors.New("Invalid query. Query is nil")
	}

	defer func() {
		query = nil
	}()

	if query.projection {
		return errors.New("Invalid query. Can't use projection queries with GetMulti")
	}

	it := query.dq.Run(ctx)

	dstv := reflect.ValueOf(dst);

	if !isValidContainer(dstv) {
		return fmt.Errorf("Invalid container of type %s. Container must be a modelable slice", dstv.Elem().Type().Name());
	}

	modelables := dstv.Elem()

	for {
		key, err := it.Next(nil)

		if err == datastore.Done {
			break;
		}

		if err != nil {
			return err
		}

		newModelable := reflect.New(query.mType)
		m, ok := newModelable.Interface().(modelable)

		if !ok {
			err = fmt.Errorf("Can't cast struct of type %s to modelable", query.mType.Name())
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
	more := false;
	rc := 0;
	it := query.dq.Run(ctx);

	dstv := reflect.ValueOf(dst);

	if !isValidContainer(dstv) {
		return nil, fmt.Errorf("Invalid container of type %s. Container must be a modelable slice", dstv.Elem().Type().Name());
	}

	modelables := dstv.Elem();

	for {

		Key, err := it.Next(nil);

		if (err == datastore.Done) {
			break;
		}

		if err != nil {
			query = nil;
			return nil, err;
		}

		more = true;
		//log.Printf("RUNNING QUERY %v FOR MODEL " + data.entityName + " - FOUND ITEM WITH KEY: " + strconv.Itoa(int(Key.IntID())), data.query);
		newModelable := reflect.New(query.mType);
		m, ok := newModelable.Interface().(modelable);

		if !ok {
			err = fmt.Errorf("Can't cast struct of type %s to modelable", query.mType.Name());
			query = nil;
			return nil, err
		}

		//todo Note: indexing here assigns the address of m to the Model.
		//this means that if a user supplied a populated dst we must reindex its elements before returning
		//or the model will point to a different modelable
		index(m);

		model := m.getModel()
		model.Key = Key;

		err = Read(ctx, m);
		if err != nil {
			query = nil;
			return nil, err;
		}
		modelables.Set(reflect.Append(modelables, reflect.ValueOf(m)));
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


//container must be *[]modelable
func isValidContainer(container reflect.Value) bool {
	if container.Kind() != reflect.Ptr {
		return false;
	}
	celv := container.Elem();
	if celv.Kind() != reflect.Slice {
		return false;
	}

	cel := celv.Type().Elem();
	ok := cel.Implements(typeOfModelable);
	if !ok {
	}
	return ok;
}

func isTypeModelable(t reflect.Type) bool {
	return t.Implements(typeOfModelable);
}

//retrieves up to datastore limits (currently 1000) entities from either memcache or datastore
//each datamap must have the Key already set

/*func (query *Query) GetMulti(ctx context.Context) ([]Model, error) {
	//check if struct contains the fields
	const batched int = 1000;

	count, err := query.Count(ctx);

	if err != nil {
		return nil, err;
	}

	div := (count / batched);
	if (count % batched != 0) {
		div++;
	}

	log.Printf("found (count) %d items. div is %v", count, div);
	//create the blueprint
	newModelable := reflect.New(q.mType);

	//allocates memory for the resulting array
	res := make([]Model, count, count);

	var chans []chan []modelable;

	//retrieve items in concurrent batches
	mutex := new(sync.Mutex);
	for paging := 0; paging < div; paging++ {
		c := make(chan []modelable);

		go func(page int, ch chan []modelable, ctx context.Context) {

			log.Printf(ctx, "Batch #%d started", page);
			offset := page * batched;

			rq := batched;
			if page + 1 == div {
				//we need the exact number o GAE will complain since len(dst) != len(keys) in getAll
				rq = count % batched;
			}

			//copy the data query into the local copy
			//dq := datastore.NewQuery(nameOfPrototype(data.Prototype()));
			dq := query.dq;
			dq = dq.Offset(offset);
			dq = dq.KeysOnly();

			keys := make([]*datastore.Key, rq, rq);
			partial := make([]modelable, rq, rq);

			done := false;

			//Lock the loop or else other goroutine will starve the go scheduler causing a datastore timeout.
			mutex.Lock();
			c := 0;
			var cursor *datastore.Cursor;
			//we first get the keys in a batch
			for !done {

				dq = dq.Limit(200);
				//right count
				if cursor != nil {
					//since we are using start, remove the offset, or it will count from the start of the query
					dq = dq.Offset(0);
					dq = dq.Start(*cursor);
				}

				it := dq.Run(ctx);

				for {

					Key, err := it.Next(nil);

					if (err == datastore.Done) {
						break;
					}

					if err != nil {
						panic(err);
					}

					dm := &dataMap{};
					*dm = *mm.dataMap;
					dm.m = reflect.New(mtype).Interface().(Prototype);

					dm.Key = Key;

					log.Printf(ctx, "c counter has value #%d. Max is %d, Key is %s", c, rq, Key.Encode());
					//populates the dst
					partial[c] = dm;
					//populate the Key
					keys[c] = Key;
					c++;
				}

				if c == rq {
					//if there are no more entries to be loaded, break the loop
					done = true;
					log.Debugf(data.context, "Batch #%d got %d keys from query", page, c);
				} else {
					//else, if we still have entries, update cursor position
					newCursor, e := it.Cursor();
					if e != nil {
						panic(err);
					}
					cursor = &newCursor;
				}
			}
			mutex.Unlock();

			fromCache, err := partial.cacheGetMulti(ctx, keys);

			if err != nil {
				log.Errorf(ctx, "Error retrieving multi from cache: %v", err);
			}

			c = 0;
			if len(fromCache) < rq {

				leftCount := len(keys) - len(fromCache);

				remainingKeys := make([]*datastore.Key, leftCount, leftCount);
				dst := make([]*dataMap, leftCount, leftCount);

				for i, k := range keys {
					_, ok := fromCache[k];
					if !ok {
						//add the pointer to those keys that have to be retrieved
						remainingKeys[c] = k;
						dst[c] = partial[i];
						c++;
					}
				}

				err = datastore.GetMulti(data.context, remainingKeys, dst);

				if err != nil {
					panic(err);
				}
			}

			log.Debugf(data.context, "Batch #%d retrieved all items. %d items retrieved from cache, %d items retrieved from datastore", page, len(fromCache), c);
			//now load the references of the model

			//todo: rework because it is not setting references.
			//			partial.cacheSetMulti(ctx);

			ch <- partial;

		} (paging, c, data.context);

		chans = append(chans, c);
	}

	offset := 0;
	for _ , c := range chans {
		partial := <- c;
		for j , dm := range partial {
			m := Model{dataMap: dm, searchable:mm.searchable};
			res[offset + j] = m;
		}

		offset += len(partial);
		close(c);
	}

	return res, nil;
}*/
