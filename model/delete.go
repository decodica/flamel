package model

import (
	"context"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/memcache"
)

func Delete(ctx context.Context, m modelable) (err error) {

	opts := datastore.TransactionOptions{}
	opts.Attempts = 1
	opts.XG = true

	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		return del(ctx, m)
	}, &opts)

	if err == nil {
		if err = deleteFromMemcache(ctx, m); err != nil && err != memcache.ErrCacheMiss {
			return err
		}
	}

	return err
}

func del(ctx context.Context, m modelable) (err error) {
	model := m.getModel()

	if model.Key == nil {
		return nil
	}

	for k := range model.references {
		ref := model.references[k]
		err = del(ctx, ref.Modelable)
		if err != nil {
			return err
		}
	}

	err = datastore.Delete(ctx, model.Key)

	return err
}
