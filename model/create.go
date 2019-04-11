package model

import (
	"context"
	"errors"
	"google.golang.org/appengine/datastore"
)

// Create methods
type CreateOptions struct {
	stringId string
	intId    int64
	attempts int
}

func NewCreateOptions() CreateOptions {
	return CreateOptions{}
}

func (opts *CreateOptions) WithStringId(id string) {
	opts.intId = 0
	opts.stringId = id
}

func (opts *CreateOptions) WithIntId(id int64) {
	opts.stringId = ""
	opts.intId = id
}

func (opts *CreateOptions) InTransaction(attempts int) {
	opts.attempts = attempts
}

func CreateWithOptions(ctx context.Context, m modelable, copts *CreateOptions) (err error) {
	index(m)

	if copts.attempts > 0 {
		opts := datastore.TransactionOptions{}
		opts.XG = true
		opts.Attempts = copts.attempts
		err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
			return createWithOptions(ctx, m, copts)
		}, &opts)
	} else {
		err = createWithOptions(ctx, m, copts)
	}

	if err == nil {
		if err = saveInMemcache(ctx, m); err != nil {
			return err
		}
	}

	return err
}

// Reads data from a modelable and writes it to the datastore as an entity with a new Key.
// Uses default options
func Create(ctx context.Context, m modelable) (err error) {
	return CreateWithOptions(ctx, m, new(CreateOptions))
}

func createWithOptions(ctx context.Context, m modelable, opts *CreateOptions) error {
	model := m.getModel()

	//if the root model has a Key then this is the wrong operation
	if model.Key != nil {
		return errors.New("data has already been created")
	}

	var ancKey *datastore.Key = nil
	//we iterate through the model references.
	//if a reference has its own Key we use it as a value in the root entity
	for i, ref := range model.references {
		rm := ref.Modelable.getModel()
		if ref.Key != nil {
			//this can't happen because we are in create, thus the root model can't have a Key
			//and can't have its reference's Key populated
			return errors.New("create called with a non-nil reference map")
		} else {
			//case is that the reference has been loaded from the datastore
			//we update the reference values using the reference Key
			//then we update the root reference map Key
			if rm.Key != nil {
				err := updateReference(ctx, &ref, rm.Key)
				if err != nil {
					return err
				}
			} else if rm.skipIfZero && isZero(ref.Modelable) {
				continue
			} else {
				err := createReference(ctx, &ref)
				if err != nil {
					return err
				}
			}
		}
		if ref.Ancestor {
			ancKey = ref.Key
		}
		model.references[i] = ref
	}

	newKey := datastore.NewKey(ctx, model.structName, opts.stringId, opts.intId, ancKey)
	key, err := datastore.Put(ctx, newKey, m)
	if err != nil {
		return err
	}
	model.Key = key

	// if the model is searchable, update the search index with the new values
	if model.searchable {
		err = searchPut(ctx, model, model.Name())
	}

	return err
}

// creates a datastore entity and stores the Key into the model field
// using default options
func createReference(ctx context.Context, ref *reference) (err error) {
	opts := NewCreateOptions()
	err = createWithOptions(ctx, ref.Modelable, &opts)

	if err != nil {
		return err
	}

	ref.Key = ref.Modelable.getModel().Key


	return nil
}
