package model

import (
	"golang.org/x/net/context"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/memcache"
	//"log"
	"fmt"
	"reflect"
)

type KeyMap map[int]string

type cacheModel struct {
	Modelable modelable
	Keys      KeyMap
}

//checks if cache Key is valid
//as per documentation Key max length is set at 250 bytes
func validCacheKey(Key string) bool {
	bs := []byte(Key)
	valid := len(bs) <= 250
	return valid
}

//Saves the modelable representation and all related references to memcache.
//It assumes that there are no stale references
func saveInMemcache(ctx context.Context, m modelable) (err error) {
	//skip unregistered models
	model := m.getModel()

	//a modelable must be registered to be saved in memcache
	if !model.isRegistered() {
		return fmt.Errorf("modelable %v is not registered", m)
	}

	if model.Key == nil {
		return nil
		// return fmt.Errorf("no key registered for modelable %s. Can't save in memcache", model.structName)
	}

	i := memcache.Item{}
	i.Key = model.EncodedKey()

	if !validCacheKey(i.Key) {
		return fmt.Errorf("cacheModel box Key %s is too long", i.Key)
	}

	keyMap := make(KeyMap)

	for _, ref := range model.references {
		r := ref.Modelable
		rm := r.getModel()
		if rm.readonly {
			continue
		}

		//throw an error if the model Key and the reference Key do not coincide
		if rm.Key == nil {
			continue
			// return fmt.Errorf("can't save to memcache. reference model Key is nil for reference: %+v", ref)
		}

		if rm.Key != ref.Key {
			return fmt.Errorf("can't save to memcache. Key of the model doesn't equal the Key of the reference for reference %+v", ref)
		}

		err = saveInMemcache(ctx, r)

		if err != nil {
			return err
		}

		if rm.Key != nil {
			keyMap[ref.idx] = rm.EncodedKey()
		}
	}

	box := cacheModel{Keys: keyMap}
	box.Modelable = m
	i.Object = box

	err = memcache.Gob.Set(ctx, &i)

	return err
}

func loadFromMemcache(ctx context.Context, m modelable) (err error) {
	model := m.getModel()

	if model.Key == nil {
		return nil
		// return fmt.Errorf("no Key registered from modelable %s. Can't load from memcache", model.structName)
	}

	cKey := model.EncodedKey()

	if !validCacheKey(cKey) {
		return fmt.Errorf("cacheModel box Key %s is too long", cKey)
	}

	box := cacheModel{Keys: make(map[int]string), Modelable: m}

	_, err = memcache.Gob.Get(ctx, cKey, &box)

	if err != nil {
		return err
	}

	for _, ref := range model.references {
		if encodedKey, ok := box.Keys[ref.idx]; ok {
			decodedKey, err := datastore.DecodeKey(encodedKey)
			if err != nil {
				return err
			}

			r := ref.Modelable
			rm := r.getModel()
			rm.Key = decodedKey

			err = loadFromMemcache(ctx, ref.Modelable)
			if err != nil {
				return err
			}
			ref.Key = decodedKey
			//assign the reference values to the box struct.
			//this needs to be done so that the passing modelable is updated
			field := reflect.Indirect(reflect.ValueOf(box.Modelable)).Field(ref.idx)
			src := reflect.Indirect(reflect.ValueOf(r))
			field.Set(src)
		} else {
			// there is no reference saved at the given key: we could be in readonly.
			// return an error and retrieve the item from datastore
			return memcache.ErrCacheMiss
		}
	}

	//if there are no error we assign the value recovered from memcache to the modelable
	defer func(error) {
		if err == nil {
			modValue := reflect.ValueOf(*model)
			dstValue := reflect.Indirect(reflect.ValueOf(m))
			srcValue := reflect.Indirect(reflect.ValueOf(box.Modelable))
			dstValue.Set(srcValue)
			//set model to the modelable Model Field
			for i := 0; i < dstValue.NumField(); i++ {
				field := dstValue.Field(i)
				fieldType := field.Type()
				if fieldType == typeOfModel {
					field.Set(modValue)
					break
				}
			}
		}
	}(err)

	return err
}

func deleteFromMemcache(ctx context.Context, m modelable) (err error) {
	model := m.getModel()

	if model.Key == nil {
		return nil
		// return fmtErrorf("no Key registered from modelable %s. Can't delete from memcache", reflect.TypeOf(m).Elem().Name())
	}

	for k, _ := range model.references {
		ref := model.references[k]
		rm := ref.Modelable.getModel()
		if rm.readonly {
			continue
		}
		err := deleteFromMemcache(ctx, ref.Modelable)
		if err != nil {
			return err
		}
		ref.Key = nil
	}

	cKey := model.EncodedKey()
	if !validCacheKey(cKey) {
		return fmt.Errorf("cacheModel box Key %s is too long", cKey)
	}

	defer func(error) {
		if err == nil {
			model.Key = nil
		}
	}(err)

	return memcache.Delete(ctx, cKey)
}
