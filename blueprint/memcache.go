package blueprint

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
	if nil == data.key {
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

		if ref.key != nil {
			//encode only not null key references.
			//if a reference has no key then its value equals the struct default value
			keyMap[k] = ref.key.Encode();
		}
	}

	cacheBox := &cachePrototype{Keys:keyMap};
	i := &memcache.Item{};
	i.Key = data.key.Encode();

	cacheBox.Proto = data.Prototype();

	i.Object = cacheBox;
	err := memcache.Gob.Set(data.context, i);
	if nil != err {
		log.Errorf(data.context, "cacheSet error setting object in memcache: %v: ", err);

	}
}

func (data *dataMap) cacheGet() error {
	if nil == data.key {
		return errors.New("Item has no key. Can't retrieve it from memcache");
	}

	skey := data.key.Encode();

	if !validCacheKey(skey) {
		panic("Exceeding cache key max capacity - call a system administrator");
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
			ref.key = decodedKey;

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

	//get items from memcache by key
	items, err := memcache.GetMulti(ctx, strKeys);

	if err != nil {
		log.Errorf(ctx, "Error in batched retrieval from memcache: %v", err);
		return nil, err;
	}

	keyMap, _ := data.KeyMap();

	//get the item in the data slice which has the key "key"
	for k, v := range keyMap {
		strKey := k.Encode();

		item, ok := items[strKey];

		if !ok {
			//if there's no key, we remove the element from the keyMap
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
				ref.key = decodedKey;
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

			if ref.key != nil {
				//encode only not null key references.
				//if a reference has no key then its value equals the struct default value
				refMap[k] = ref.key.Encode();
			}

		}

		//prepare the cachebox representation of the model
		cacheBox := &cachePrototype{};
		cacheBox.Keys = refMap;
		cacheBox.Proto = pvalue.Prototype();

		i := &memcache.Item{};
		i.Key = pvalue.key.Encode();
		i.Object = cacheBox;

		items[c] = i;
		c++;
	}

	err := memcache.Gob.SetMulti(ctx, items);
	if err != nil {
		log.Errorf(ctx, "Couldn't batch save items in cache: %v", err);
	}

	return err;
}

//checks if cache key is valid
func validCacheKey(key string) bool {
	valid := len(key) <= 250;
	return valid;
}*/
