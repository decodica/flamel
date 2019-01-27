package model

import (
	"context"
	"google.golang.org/appengine/datastore"
)

// Reads data from the datastore and writes them into the modelable.
func Read(ctx context.Context, m modelable) (err error) {
	index(m)

	opts := datastore.TransactionOptions{}
	opts.XG = true
	opts.Attempts = 1

	err = loadFromMemcache(ctx, m)

	if err == nil {
		return nil
	}

	// else we ignore the memcache result and we read from datastore

	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		return read(ctx, m)
	}, &opts)

	return err
}

func read(ctx context.Context, m modelable) error {
	model := m.getModel()

	if model.Key == nil {
		return nil
	}

	err := datastore.Get(ctx, model.Key, m)

	if err != nil {
		return err
	}

	for k, ref := range model.references {
		rm := ref.Modelable.getModel()
		err := read(ctx, ref.Modelable)
		if err != nil {
			return err
		}
		ref.Key = rm.Key
		model.references[k] = ref
	}

	return nil
}
