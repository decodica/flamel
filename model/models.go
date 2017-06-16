package model

import (
	"golang.org/x/net/context"
	"reflect"
	"fmt"
	"errors"
	"google.golang.org/appengine/datastore"
)

//Batch version of Read.
//Can't be run in a transaction because of too many entities group.
//It can return a datastore multierror.
//todo: EXPERIMENTAL - USE AT OWN RISK
func ReadMulti(ctx context.Context, dst interface{}) error {
	return readMulti(ctx, dst);
}

//Batch version of read. It wraps datastore.GetMulti and adapts it to the modelable fw
func readMulti(ctx context.Context, dst interface{}) error {

	dstv := reflect.ValueOf(dst);

	mod := modelOf(dst);
	if mod == nil {
		return errors.New("Can't determine model of provided dst.");
	}

	//get the array the slice points to
	collection := dstv;
	//save the references indexes
	refsi := make([]int, 0, 0);
	for k, _ := range mod.references {
		refsi = append(refsi, k);
	}
	//populate the key slice
	l := collection.Len();
	keys := make([]*datastore.Key, collection.Len(), l);

	for i := 0; i < l; i++ {
		mble, ok := collection.Index(i).Interface().(modelable);
		if !ok {
			return fmt.Errorf("Invalid container of type %s. Container must be a slice of modelables", dstv.Elem().Type().Name());
		}

		if mble.getModel().Key == nil {
			return fmt.Errorf("Missing key for modelable at index %d", i);
		}

		keys[i] = mble.getModel().Key;
	}

	err := datastore.GetMulti(ctx, keys, dst);

	if err != nil {
		return err;
	}

	for _, v := range refsi {
		//allocate a slice and fill it with pointers of the entities retrieved
		typ := reflect.TypeOf(mod.references[v].Modelable);
		refs := reflect.MakeSlice(reflect.SliceOf(typ), l, l);
		for i := 0; i < l; i++ {
			reflref := collection.Index(i).Elem().Field(v);
			refs.Index(i).Set(reflref.Addr());
		}
		err := readMulti(ctx, refs.Interface());
		if err != nil {
			return err;
		}
	}

	return nil;
}
