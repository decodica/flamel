package model

import (
	"cloud.google.com/go/datastore"
	"context"
	"google.golang.org/appengine/log"
)

type ReadOptions struct {
	attempts int
}

func NewReadOptions() ReadOptions {
	return ReadOptions{}
}

func (opts *ReadOptions) InTransaction(attempts int) {
	opts.attempts = attempts
}

func Read(ctx context.Context, m modelable) (err error) {
	index(m)

	err = loadFromMemcache(ctx, m)
	if err == nil {
		return nil
	}

	err = read(ctx, m)
	if err == nil {
		if err = saveInMemcache(ctx, m); err != nil {
			log.Warningf(ctx, "error saving modelable %s to memcache: %s", m.getModel().Name(), err.Error())
		}
	}
	return err
}

// Reads data from the datastore and writes them into the modelable.
func ReadInTransaction(ctx context.Context, m modelable, opts *ReadOptions) (err error) {
	index(m)

	err = loadFromMemcache(ctx, m)

	if err == nil {
		return nil
	}

	to := datastore.MaxAttempts(opts.attempts)
	// else we ignore the memcache result and we read from datastore
	client := ClientFromContext(ctx)
	_, err = client.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		return read(ctx, m)
	}, to, datastore.ReadOnly)

	if err == nil {
		if err := saveInMemcache(ctx, m); err != nil {
			log.Warningf(ctx, "error saving modelable %s to memcache: %s", m.getModel().Name(), err.Error())
		}
	}
	return err
}

func read(ctx context.Context, m modelable) error {
	model := m.getModel()

	if model.Key == nil {
		return nil
	}

	client := ClientFromContext(ctx)
	err := client.Get(ctx, model.Key, m)

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
