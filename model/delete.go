package model

import (
	"cloud.google.com/go/datastore"
	"context"
	"fmt"
	"google.golang.org/appengine/memcache"
	"reflect"
)

// recursively deletes a modelable and all its references
func Clear(ctx context.Context, m modelable) (err error) {

	client := ClientFromContext(ctx)
	opts := datastore.MaxAttempts(1)
	_, err = client.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		return clear(ctx, m)
	}, opts)

	if err == nil {
		if err = deleteFromMemcache(ctx, m); err != nil && err != memcache.ErrCacheMiss {
			return err
		}
	}

	return err
}

func clear(ctx context.Context, m modelable) (err error) {
	model := m.getModel()

	if model.Key == nil {
		return nil
	}

	for k := range model.references {
		ref := model.references[k]
		rm := ref.Modelable.getModel()
		if rm.readonly {
			continue
		}

		err = clear(ctx, ref.Modelable)
		if err != nil {
			return err
		}
	}
	client := ClientFromContext(ctx)
	err = client.Delete(ctx, model.Key)

	return err
}

// deletes a single reference
func Delete(ctx context.Context, ref modelable, parent modelable) (err error) {

	child := ref.getModel()
	if child.Key == nil {
		return fmt.Errorf("reference %s has a nil key", child.Name())
	}

	client := ClientFromContext(ctx)
	err = client.Delete(ctx, child.Key)
	if err == nil {

		if child.searchable {
			if err := searchDelete(ctx, child, child.Name()); err != nil {
				return err
			}
		}

		if err = deleteFromMemcache(ctx, child); err != nil && err != memcache.ErrCacheMiss {
			return err
		}
	}

	if parent == nil {
		return nil
	}

	// handle the case where the reference is single
	index(parent)

	idx := -1
	for _, c := range parent.getModel().references {
		if c.Modelable == ref {
			idx = c.idx
			break
		}
	}

	if idx == -1 {
		return fmt.Errorf("%s is not a reference of %s", ref.getModel().Name(), parent.getModel().Name())
	}

	ctype := reflect.TypeOf(ref).Elem()
	newref := reflect.New(ctype).Interface().(modelable)

	pv := reflect.ValueOf(parent).Elem()
	pv.Field(idx).Set(reflect.ValueOf(newref).Elem())

	_, err = client.Put(ctx, parent.getModel().Key, parent)
	if err != nil {
		return err
	}

	index(parent)

	return saveInMemcache(ctx, parent)
}
