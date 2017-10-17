package model

import (
	"golang.org/x/net/context"
	"google.golang.org/appengine/memcache"
	"google.golang.org/appengine/datastore"
	//"log"
	"fmt"
	"reflect"
)

type KeyMap map[int]string;

type cacheModel struct {
	Modelable modelable
	Keys KeyMap
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
	if !model.registered {
		return fmt.Errorf("modelable %v is not registered", m)
	}

	if nil == model.Key {
		return fmt.Errorf("no Key registered for modelable %s. Can't save in memcache.", model.structName);
	}

	i := memcache.Item{}
	i.Key = model.EncodedKey()

	if !validCacheKey(i.Key) {
		return fmt.Errorf("cacheModel box Key %s is too long", i.Key)
	}

	keyMap := make(KeyMap)

	for k, _ := range model.references {
		ref := model.references[k]
		r := ref.Modelable
		rm := r.getModel()

		//throw an error if the model Key and the reference Key do not coincide
		if rm.Key == nil {
			return fmt.Errorf("can't save to memcache. reference model Key is nil for reference: %+v", ref)
		}

		if rm.Key != ref.Key {
			return fmt.Errorf("can't save to memcache. Key of the model doesn't equal the Key of the reference for reference %+v", ref)
		}

		err = saveInMemcache(ctx, r)

		if err != nil {
			return err
		}

		if rm.Key != nil {
			keyMap[k] = rm.EncodedKey()
		}
	}

	box := cacheModel{Keys:keyMap}
	box.Modelable = m
	i.Object = box

	err = memcache.Gob.Set(ctx, &i)

	return err
}

func loadFromMemcache(ctx context.Context, m modelable) (err error) {
	model := m.getModel()

	if model.Key == nil {
		return fmt.Errorf("no Key registered from modelable %s. Can't load from memcache", model.structName)
	}

	cKey := model.EncodedKey();

	if !validCacheKey(cKey) {
		return fmt.Errorf("cacheModel box Key %s is too long", cKey)
	}

	box := cacheModel{Keys:make(map[int]string), Modelable:m}

	_, err = memcache.Gob.Get(ctx, cKey, &box)

	if err != nil {
		return err
	}

	for k, _ := range model.references {
		if encodedKey, ok := box.Keys[k]; ok {
			decodedKey, err := datastore.DecodeKey(encodedKey)
			if err != nil {
				return err
			}
			ref := model.references[k]
			r := ref.Modelable
			rm := r.getModel()
			rm.Key = decodedKey
			err = loadFromMemcache(ctx, ref.Modelable)
			if err != nil {
				return err
			}
			ref.Key = decodedKey
			model.references[k] = ref
			//assign the reference values to the box struct.
			//this needs to be done so that the passing modelable is updated
			field := reflect.Indirect(reflect.ValueOf(box.Modelable)).Field(k)
			src := reflect.Indirect(reflect.ValueOf(r))
			field.Set(src)
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
		return fmt.Errorf("no Key registered from modelable %s. Can't delete from memcache", reflect.TypeOf(m).Elem().Name())
	}

	for k, _ := range model.references {
		ref := model.references[k]
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

/*func (data) cacheGet() error {
	if nil == data.Key {
		return errors.New("Item has no Key. Can't retrieve it from memcache");
	}

	skey := data.Key.Encode();

	if !validCacheKey(skey) {
		panic("Exceeding cache Key max capacity - call a system administrator");
	}

	keyMap := make(map[int]string);
	cacheBox := &cachePrototype{Keys:keyMap};

	cacheBox.Proto = data.Prototype();
	_, err := memcache.Gob.Get(data.context, skey, cacheBox);

	if err != nil {
		return err;
	}

	data.m = cacheBox.Proto;

	for k, _ := range data.references {
		if encodedKey, ok := cacheBox.Keys[k]; ok {
			decodedKey, e := datastore.DecodeKey(encodedKey);
			if e != nil {
				panic(e);
			}
			ref := data.references[k];
			ref.Key = decodedKey;

			err = ref.read();

			if err != nil {
				return err;
			}
		}
	}
	return err;
}*/

/*import (
	"reflect"
	"google.golang.org/appengine/memcache"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
	"errors"
	"golang.org/x/net/context"
)

type cachePrototype struct {
	Proto Prototype
	//maps field position to ref keys
	Keys map[int]string

}

func (data dataMap) cacheSet() {
	if nil == data.Key {
		return;
	}

	protoValue := reflect.ValueOf(data.Prototype()).Elem();

	keyMap := make(map[int]string);

	for k, _ := range data.references {

		ref := data.references[k];
		//this works since only structs are supported as references
		//todo:support slices as well
		val := protoValue.Field(k);
		reflect.ValueOf(ref.Prototype()).Elem().Set(val);

		if ref.Key != nil {
			//encode only not null Key references.
			//if a reference has no Key then its value equals the struct default value
			keyMap[k] = ref.Key.Encode();
		}
	}

	cacheBox := &cachePrototype{Keys:keyMap};
	i := &memcache.Item{};
	i.Key = data.Key.Encode();

	cacheBox.Proto = data.Prototype();

	i.Object = cacheBox;
	err := memcache.Gob.Set(data.context, i);
	if nil != err {
		log.Errorf(data.context, "cacheSet error setting object in memcache: %v: ", err);

	}
}

func (data *dataMap) cacheGet() error {
	if nil == data.Key {
		return errors.New("Item has no Key. Can't retrieve it from memcache");
	}

	skey := data.Key.Encode();

	if !validCacheKey(skey) {
		panic("Exceeding cache Key max capacity - call a system administrator");
	}

	keyMap := make(map[int]string);
	cacheBox := &cachePrototype{Keys:keyMap};

	cacheBox.Proto = data.Prototype();
	_, err := memcache.Gob.Get(data.context, skey, cacheBox);

	if err != nil {
		return err;
	}

	data.m = cacheBox.Proto;

	for k, _ := range data.references {
		if encodedKey, ok := cacheBox.Keys[k]; ok {
			decodedKey, e := datastore.DecodeKey(encodedKey);
			if e != nil {
				panic(e);
			}
			ref := data.references[k];
			ref.Key = decodedKey;

			err = ref.read();

			if err != nil {
				return err;
			}
		}
	}
	return err;
}

//returns a map with keys and values retrieved from memcache
//todo: retrieved items should substitute data's items
func (data dataMaps) cacheGetMulti(ctx context.Context, keys []*datastore.Key) (map[*datastore.Key]*dataMap, error) {

	//convert the keys to a string array. Keep the index untouched
	strKeys := make([]string, len(keys), len(keys));

	for i, v := range keys {
		strKeys[i] = v.Encode();
	}

	//get items from memcache by Key
	items, err := memcache.GetMulti(ctx, strKeys);

	if err != nil {
		log.Errorf(ctx, "Error in batched retrieval from memcache: %v", err);
		return nil, err;
	}

	keyMap, _ := data.KeyMap();

	//get the item in the data slice which has the Key "Key"
	for k, v := range keyMap {
		strKey := k.Encode();

		item, ok := items[strKey];

		if !ok {
			//if there's no Key, we remove the element from the keyMap
			log.Debugf(ctx, "Key %s not found in map", strKey);
			delete(keyMap, k);
			continue;
		}

		//create the object to translate from gob
		cacheMap := make(map[int]string);
		cacheBox := &cachePrototype{};

		cacheBox.Keys = cacheMap;
		cacheBox.Proto = v.Prototype();

		err = memcache.Gob.Unmarshal(item.Value, cacheBox);

		if err != nil {
			return keyMap, err;
		}

		v.m = cacheBox.Proto;

		for k, _ := range v.references {
			if encodedKey, ok := cacheBox.Keys[k]; ok {
				decodedKey, e := datastore.DecodeKey(encodedKey);
				if e != nil {
					panic(e);
				}
				ref := v.references[k];
				ref.Key = decodedKey;
			}
		}
	}

	log.Debugf(ctx, "Retrieved %d items from memcache in batched mode", len(keyMap));
	return keyMap, err;
}

func (data dataMaps) cacheSetMulti(ctx context.Context) error {
	keyMap, _ := data.KeyMap();

	//prepare items to be set into memcache
	items := make([]*memcache.Item, len(keyMap), len(keyMap));

	c := 0;
	for _, v := range keyMap {

		//get the value so we avoid concurrency issues
		pvalue := *v;

		protoValue := reflect.ValueOf(pvalue.Prototype()).Elem();

		refMap := make(map[int]string);

		for k, _ := range refMap {

			ref := pvalue.references[k];
			//this works since only structs are supported as references
			//todo:support slices as well
			val := protoValue.Field(k);
			reflect.ValueOf(ref.Prototype()).Elem().Set(val);

			if ref.Key != nil {
				//encode only not null Key references.
				//if a reference has no Key then its value equals the struct default value
				refMap[k] = ref.Key.Encode();
			}

		}

		//prepare the cachebox representation of the model
		cacheBox := &cachePrototype{};
		cacheBox.Keys = refMap;
		cacheBox.Proto = pvalue.Prototype();

		i := &memcache.Item{};
		i.Key = pvalue.Key.Encode();
		i.Object = cacheBox;

		items[c] = i;
		c++;
	}

	err := memcache.Gob.SetMulti(ctx, items);
	if err != nil {
		log.Errorf(ctx, "Couldn't batch save items in cache: %v", err);
	}

	return err;
}*/
