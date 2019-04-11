package model

import (
	"context"
	"fmt"
	"google.golang.org/appengine/datastore"
)

// Reads data from a modelable and writes it into the corresponding entity of the datastore.
// If a reference is read from the storage and then assigned to the root modelable
// the root modelable will point to the loaded entity
// If a reference is newly created its value will be updated accordingly to the model
func UpdateInTransaction(ctx context.Context, m modelable) (err error) {
	index(m)

	opts := datastore.TransactionOptions{}
	opts.XG = true
	opts.Attempts = 1
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		return update(ctx, m)
	}, &opts)

	if err == nil {
		if err = saveInMemcache(ctx, m); err != nil {
			return err
		}
	}

	return err
}

func Update(ctx context.Context, m modelable) error {
	index(m)

	err := update(ctx, m)

	if err == nil {
		if err = saveInMemcache(ctx, m); err != nil {
			return err
		}
	}

	return err
}

func updateReference(ctx context.Context, ref *reference, key *datastore.Key) (err error) {
	model := ref.Modelable.getModel()

	// align model key with its parent ref key
	model.Key = key
	ref.Key = key

	if model.readonly {
		return nil
	}

	//we iterate through the references of the current model
	for i, r := range model.references {
		rm := r.Modelable.getModel()
		//We check if the parent has a Key related to the reference.
		//If it does we use the Key provided by the parent to update the children
		if r.Key != nil {
			err := updateReference(ctx, &r, r.Key)
			if err != nil {
				return err
			}
		} else {
			//else, if the parent doesn't have the Key we must check the children
			if rm.Key != nil {
				//the child was loaded and then assigned to the parent: update the children
				//and make the parent point to it
				err := updateReference(ctx, &r, rm.Key)
				if err != nil {
					return err
				}
			} else if rm.skipIfZero && isZero(r.Modelable) {
				// the child is empty and must be kept empty
				continue
			} else {
				//neither the parent and the children specify a Key.
				//We create the children and update the parent's Key
				err := createReference(ctx, &r)
				if err != nil {
					return err
				}
			}
		}
		model.references[i] = r
	}

	_, err = datastore.Put(ctx, key, ref.Modelable)

	if err != nil {
		return err
	}

	// if the model is searchable, update the search index with the new values
	if model.searchable {
		err = searchPut(ctx, model, model.Name())
	}
	return err
}

// updates the given modelable
// iterates through the modelable reference.
// if the reference has a Key
func update(ctx context.Context, m modelable) error {
	model := m.getModel()

	if model.Key == nil {
		return fmt.Errorf("can't update modelable %v. Missing Key", m)
	}

	for i, ref := range model.references {
		rm := ref.Modelable.getModel()

		if rm.Key != nil {
			err := updateReference(ctx, &ref, rm.Key)
			if err != nil {
				return err
			}
		} else if ref.Key != nil {
			// in this case a new reference has been assigned in place of an empty reference
			err := updateReference(ctx, &ref, ref.Key)
			if err != nil {
				return err
			}
		} else if rm.skipIfZero && isZero(ref.Modelable) {
			// skip if the ref must be kept empty
			continue
		} else {
			// else create it
			err := createReference(ctx, &ref)
			if err != nil {
				return err
			}
		}

		model.references[i] = ref
	}

	Key, err := datastore.Put(ctx, model.Key, m)

	if err != nil {
		return err
	}

	model.Key = Key

	if model.searchable {
		err = searchPut(ctx, model, model.Name())
	}

	return nil
}
