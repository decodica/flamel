package model

import (
	"errors"
	"fmt"
	"golang.org/x/net/context"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/memcache"
	"reflect"
)

//Batch version of Read.
//Can't be run in a transaction because of too many entities group.
//It can return a datastore multierror.
//todo: EXPERIMENTAL - USE AT OWN RISK
func ReadMulti(ctx context.Context, dst interface{}) error {
	return readMulti(ctx, dst)
}

type source byte

const (
	none source = iota + 1
	cache
	store
)

//Batch version of read. It wraps datastore.GetMulti and adapts it to the modelable fwk
func readMulti(ctx context.Context, dst interface{}) error {

	collection := reflect.ValueOf(dst)

	if collection.Kind() != reflect.Slice {
		return fmt.Errorf("invalid container: container kind must be slice. Kind %s provided", collection.Kind())
	}

	mod := modelOf(dst)
	if mod == nil {
		return errors.New("can't determine model of provided dst")
	}

	//get the array the slice points to

	//save the references indexes
	refsi := make([]int, 0, 0)
	for _, ref := range mod.references {
		refsi = append(refsi, ref.idx)
	}
	//populate the key slice
	l := collection.Len()

	keys := make([]*datastore.Key, 0, collection.Cap())

	// make a copy of the destination slice
	destination := reflect.MakeSlice(collection.Type(), 0, collection.Cap())

	for i := 0; i < l; i++ {
		mble, ok := collection.Index(i).Interface().(modelable)
		if !ok {
			return fmt.Errorf("invalid container of type %s. Container must be a slice of modelables", collection.Elem().Type().Name())
		}

		// try to fetch from memcache
		err := loadFromMemcache(ctx, mble)
		if err == nil {
			collection.Index(i).Set(reflect.ValueOf(mble))
			continue
		}

		if err != memcache.ErrCacheMiss {
			log.Warningf(ctx, "error retrieving model %s from memcache: %s", mble.getModel().Name(), err.Error())
		}

		// we have an empty ref, skip it
		if mble.getModel().Key == nil {
			continue
		}

		keys = append(keys, mble.getModel().Key)
		destination = reflect.Append(destination, collection.Index(i))
	}

	// debug
	di := destination.Interface()
	// we retrieved everything from memcache, no need to call datastore
	if len(keys) > 0 {
		err := datastore.GetMulti(ctx, keys, di)

		if err != nil {
			return err
		}
	}

	for j, ref := range mod.references {
		//allocate a slice and fill it with pointers of the entities retrieved
		typ := reflect.TypeOf(ref.Modelable)
		refs := reflect.MakeSlice(reflect.SliceOf(typ), l, l)
		for i := 0; i < l; i++ {
			reflref := collection.Index(i).Elem().Field(ref.idx)
			// set the slice as the destination for the reference read
			refs.Index(i).Set(reflref.Addr())
			tmodel := collection.Index(i).Interface().(modelable)
			tmodel.getModel().references[j].Key = refs.Index(i).Interface().(modelable).getModel().Key
		}
		// read into the address of the newly allocated references
		err := readMulti(ctx, refs.Interface())
		if err != nil {
			return err
		}
	}

	return nil
}
