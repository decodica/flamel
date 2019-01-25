package model

import (
	"errors"
	"fmt"
	"golang.org/x/net/context"
	"google.golang.org/appengine/datastore"
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
	for k, _ := range mod.references {
		refsi = append(refsi, k)
	}
	//populate the key slice
	l := collection.Len()

	source := make([]source, collection.Len(), l)
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
			source[i] = cache
			continue
		}

		if err != memcache.ErrCacheMiss {
			return fmt.Errorf("can't fetch modelable %s from cache", mble.getModel().Name())
		}

		// we have an empty ref, skip it
		if mble.getModel().Key == nil {
			source[i] = none
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

	for _, v := range refsi {
		//allocate a slice and fill it with pointers of the entities retrieved
		typ := reflect.TypeOf(mod.references[v].Modelable)
		refs := reflect.MakeSlice(reflect.SliceOf(typ), l, l)
		for i := 0; i < l; i++ {
			reflref := collection.Index(i).Elem().Field(v)
			refs.Index(i).Set(reflref.Addr())
		}
		err := readMulti(ctx, refs.Interface())
		if err != nil {
			return err
		}
	}

	return nil
}
