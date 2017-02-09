package model

import (
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine"
	gaelog "google.golang.org/appengine/log"
	"reflect"
	"time"
	"golang.org/x/net/context"
	"log"
	"fmt"
	"errors"
	"strings"
	"encoding/gob"
)

var (
	typeOfBlobKey = reflect.TypeOf(appengine.BlobKey(""))
	typeOfByteSlice = reflect.TypeOf([]byte(nil))
	typeOfByteString = reflect.TypeOf(datastore.ByteString(nil))
	typeOfGeoPoint = reflect.TypeOf(appengine.GeoPoint{})
	typeOfTime = reflect.TypeOf(time.Time{})
)

const ref_model_prefix string = "ref_";

const model_modelable_field_name string = "modelable";

const val_serparator string = ".";

const tag_domain string = "model";

const default_entry_count_per_read_batch int = 500;



const tag_skip string = "-";
const tag_search string = "search";
const tag_noindex string = "noindex";

type modelable interface {
	getModel() *Model
	setModel(m Model)
}

type Model struct {
	//Note: this is necessary to allow simple implementation of memcache encoding and coding
	//else we get the all unexported fields error from Gob package
	Registered bool `model:"-"`
	//*dataMap
	/*search.FieldLoadSaver
	searchQuery string
	searchable bool*/
	//represents the mapping of the modelable containing this Model
	*structure
	//it maps field with field position and keeps the record
	//propertyLoader `model:"-"`

	key *datastore.Key

	//the embedding modelable
	modelable modelable `model:"-"`
}

type structure struct {
	//encoded struct represents the mapping of the struct
	*encodedStruct
	references map[int]modelable
}

type Prototype interface {
	datastorable
}

type datastorable interface {
	create() error
	read() error
	update() error
	delete() error
}

//Model satisfies modelable
//Returns the current Model.
func (model *Model) getModel() *Model {
	return model;
}

//Set the value of m into the Model
func (model *Model) setModel(m Model) {
	*model = m;
}

//returns -1 if the model doesn't have an id
//returns the id of the model otherwise
func (model Model) Id() int64 {
	if model.key == nil {
		return -1;
	}

	return model.key.IntID();
}

//Returns the name of the modelable this model refers to
func (model Model) Name() string {
	return model.structName;
}

func (model Model) EncodedKey() string {
	if model.key == nil {
		return "";
	}

	return model.key.Encode();
}


//Registers m and its references to work with the model framework.
//Zeroed struct references won't be written to/read from the datastore.
func register(m modelable) {

	mType := reflect.TypeOf(m).Elem()
	//retrieve modelable anagraphics
	obj := reflect.ValueOf(m).Elem()
	name := mType.Name()

	var s structure;
	//check if the modelable structure has been already mapped

	if enStruct, ok := encodedStructs[mType]; ok {
		s.encodedStruct = enStruct;
	} else {
		//map the struct
		s.encodedStruct = newEncodedStruct()
		mapStructure(mType, s.encodedStruct, name)
	}

	s.structName = name;

	//log.Printf("Modelable struct has name %s", s.structName);
	s.references = make(map[int]modelable)
	//map the references of the model
	for i := 0; i < obj.NumField(); i++ {

		fType := mType.Field(i);

		if obj.Field(i).Type() == typeOfModel {
			//skip mapping of the model
			continue
		}

		tags := strings.Split(fType.Tag.Get(tag_domain), ",")
		tagName := tags[0]

		if tagName == tag_skip {
			log.Printf("Field %s is skippable.", fType.Name)
			continue
		}

		if tagName == tag_search {
			//todo
		}

		if obj.Field(i).Kind() == reflect.Struct {

			if !obj.Field(i).CanAddr() {
				panic(fmt.Errorf("Unaddressable reference %v in Model", obj.Field(i)));
			}

			if reference, ok := obj.Field(i).Addr().Interface().(modelable); ok {
				//we register the modelable
				register(reference)
				//here the reference is registered
				s.references[i] = reference
			}
		}
	}

	if !m.getModel().Registered {
		model := Model{structure: &s}
		model.modelable = m;
		model.Registered = true;
		model.key = m.getModel().key;
		m.setModel(model)
		gob.Register(model.modelable);
	}

}

func (model *Model) Save() ([]datastore.Property, error) {
	return toPropertyList(model.modelable);
}

func (model *Model) Load(props []datastore.Property) error {
	return fromPropertyList(model.modelable, props);
}

//creates a datastore entity anmd stores the key into the model field
func create(ctx context.Context, m modelable) error {
	model := m.getModel();

	if (model.key != nil) {
		return errors.New("data has already been created");
	}

	err := createOrUpdateReferences(ctx, model);
	if err != nil {
		return err;
	}

	incompleteKey := datastore.NewIncompleteKey(ctx, model.structName, nil);
	log.Printf(">>>>> Incomplete for struct %s key is %s ", model.structName, incompleteKey.String())

	key, err := datastore.Put(ctx, incompleteKey, m);

	if err != nil {
		return err;
	}

	model.key = key;

	return nil;
}

func update(ctx context.Context, m modelable) error {
	model := m.getModel();

	if model.key == nil {
		return fmt.Errorf("Can't update modelable %v. Missing key", m);
	}

	err := createOrUpdateReferences(ctx, model);
	if err != nil {
		return err;
	}

	key, err := datastore.Put(ctx, model.key, m);

	if err != nil {
		return err;
	}

	model.key = key;

	return nil;
}

//Returns true if the modelable is zero.
//todo: benchmark and possibly make more efficient
func isZero(ref modelable) bool {
	typ := reflect.TypeOf(ref).Elem();
	zero := reflect.New(typ).Interface().(modelable);
	zero.setModel(*ref.getModel());
	//log.Printf("Type is %s, Zero is %+v, value is %+v. Ref is zero %v",typ.Name(), zero, ref, reflect.DeepEqual(zero, ref));
	return  reflect.DeepEqual(zero, ref);
}


//creates or updates references of model model.
//if the reference is zero there are two behaviours:
//in create the reference is skipped
//in update the reference is deleted
func createOrUpdateReferences(ctx context.Context, model *Model) error {
	for k, _ := range model.references {
		ref := model.references[k];
		refModel := ref.getModel();

		if refModel.key == nil {
			//skip the zero reference
			if isZero(ref) {
				return nil;
			}
			err := create(ctx, ref);
			if err != nil {
				gaelog.Errorf(ctx, "Transaction failed when creating reference %s. Error %s", model.structName, err.Error())
				return err;
			}
		} else {
		/*	if isZero(ref) {
				return delete(ctx, ref);
			}*/
			err := update(ctx, ref);
			if err != nil {
				gaelog.Errorf(ctx, "Transaction failed when updating reference %s. Error %s", model.structName, err.Error())
				return err
			}
		}
	}

	return nil;
}

//Reads data from a modelable and writes it to the datastore as an entity with a new key.
func Create(ctx context.Context, m modelable) (err error) {
	if !m.getModel().Registered {
		register(m);
	}

	defer func() {
		if err == nil {
			err = saveInMemcache(ctx, m)
			if err != nil {
				gaelog.Errorf(ctx, "Error saving items in memcache: %v", err);
			}
		}
	}();

	opts := datastore.TransactionOptions{}
	opts.XG = true;
	opts.Attempts = 1;
	err = datastore.RunInTransaction(ctx, func (ctx context.Context) error {
		return create(ctx, m);
	}, &opts)

	return err;
}

//Reads data from a modelable and writes it into the corresponding entity of the datastore.
//If m is unregistered it will panic
//In update operations unregistered references won't overwrite previous stored values.
//As an example registering a modelable, change its reference, register the modelable again and
// calling Update will cause references to be written twice: one for the first registered ref and the other for the updated reference.
func Update(ctx context.Context, m modelable) (err error) {
	if !m.getModel().Registered {
		register(m);
	}

	defer func() {
		if err == nil {
			err = saveInMemcache(ctx, m)
			if err != nil {
				gaelog.Errorf(ctx, "Error saving items in memcache: %v", err);
			}
		}
	}();

	opts := datastore.TransactionOptions{}
	opts.XG = true;
	opts.Attempts = 1;
	err = datastore.RunInTransaction(ctx, func (ctx context.Context) error {
		return update(ctx, m);
	}, &opts)

	return err
}

//Loads values from the datastore for the entity with the given id.
//Entity types must be the same with m and the entity whos id is id
func ModelableFromID(ctx context.Context, m modelable, id int64) error {
	//first try to retrieve item from memcache
	model := m.getModel();
	if !model.Registered {
		register(m);
	}
	model.key = datastore.NewKey(ctx, model.structName, "", id, nil);
	return Read(ctx, m);
}

func read(ctx context.Context, m modelable) error {
	model := m.getModel();

	if model.key == nil {
		return errors.New(fmt.Sprintf("Can't populate struct %s. Model has no key", model.structName));
	}

	err := datastore.Get(ctx, model.key, m)

	if err != nil {
		return err;
	}

	for k, _ := range model.references {
		ref := model.references[k];
		log.Printf("Populating modelable %+v, reference of modelable %+v", ref, m);
		err := read(ctx, ref);
		if err != nil {
			return err;
		}
	}

	return nil
}

//Reads data from the datastore and writes them into the modelable.
//Writing into a modelable can happen only if the modelable is registered and has an ID.
func Read(ctx context.Context, m modelable) (err error) {
	if !m.getModel().Registered {
		register(m);
	}

	opts := datastore.TransactionOptions{}
	opts.XG = true;
	opts.Attempts = 1;

	err = loadFromMemcache(ctx, m);

	if err == nil {
		return err
	}

	err = datastore.RunInTransaction(ctx, func (ctx context.Context) error {
		return read(ctx, m);
	}, &opts)

	return err;
}

func delete(ctx context.Context, m modelable) (err error) {
	model := m.getModel();

	if model.key == nil {
		return fmt.Errorf("Can't delete modelable %s. Missing key.", reflect.TypeOf(model).Name());
	}

	for k, _ := range model.references {
		ref := model.references[k];
		err = delete(ctx, ref);
		if err != nil {
			return err;
		}
	}

	err = datastore.Delete(ctx, model.key);

	if err != nil {
		model.key = nil;
	}

	return err;
}

func Delete(ctx context.Context, m modelable) (err error) {

	defer func() {
		if err == nil {
			err = deleteFromMemcache(ctx, m)
			if err != nil {
				gaelog.Errorf(ctx, "Error deleting items from memcache: %v", err);
			}
		}
	}();

	opts := datastore.TransactionOptions{}
	opts.Attempts = 1;
	opts.XG = true;

	err = datastore.RunInTransaction(ctx, func (ctx context.Context) error {
		return delete(ctx, m);
	}, &opts);

	return err;
}

/*
func nameOfModelable(m modelable) string {
	return reflect.ValueOf(m).Elem().Type().String();
}

func makeRefname(base string) string {
	return ref_model_prefix + base;
}

//also sets the model to nil
func (data *dataMap) delete() error {
	if (data.key == nil) {
		return errors.New("Can't delete a data that hasn't been loaded or created");
	}

	err := datastore.Delete(data.context, data.key);

	if err != nil {
		return err;
	}

	defer func(err error) {
		if err == nil {
			memcache.Delete(data.context, data.key.Encode());
			data.Print("==== MEMCACHE ==== DELETE " + data.entityName + " FROM MEMCACHE");
		}
	}(err);

	return err;
}

func (model *Model) Delete() error {
	e := model.dataMap.delete();

	defer func(err error) {
		if err == nil {
			e := model.deleteSearch();
			if e == nil {
				model = nil;
			}
		}
	}(e);

	return e;
}

//retrieves up to datastore limits (currently 1000) entities from either memcache or datastore
//each datamap must have the key already set
func (data dataMaps) readMulti(ctx context.Context) {

	//get the model keys
	keys := data.Keys();
	fromCache, err := data.cacheGetMulti(ctx, keys);

	if err != nil {
		log.Errorf(ctx, "Error retrieving multi from cache: %v", err);
	}

	c := 0;
	if len(fromCache) < data.Len() {

		leftCount := len(keys) - len(fromCache);

		remainingKeys := make([]*datastore.Key, leftCount, leftCount);
		dst := make([]*dataMap, leftCount, leftCount);

		for i, k := range keys {
			_, ok := fromCache[k];
			if !ok {
				//add the pointer to those keys that have to be retrieved
				remainingKeys[c] = k;
				dst[c] = data[i];
				c++;
			}
		}

		err = datastore.GetMulti(ctx, remainingKeys, dst);

		if err != nil {
			panic(err);
		}
	}

	//now load the references of the model
	data.cacheSetMulti(ctx);
}

//quick key value storage that allows saving of model key to memcache.
//this allows for strong consistence storage.

func (model *Model) SaveToKeyValue(key string) error {
	i := &memcache.Item{};
	i.Key = key;
	i.Value = []byte(model.key.Encode());
	err := memcache.Set(model.context, i);

	if err == nil {
		err = model.Update();
	}

	return err;
}

func (model *Model) LoadFromKeyValue(key string) error {
	//recupero key da token in memcache
	item, e := memcache.Get(model.context, key);

	if e != nil {
		return e;
	}

	userKey := string(item.Value);
	model.key, _ = datastore.DecodeKey(userKey);

	e = datastore.Get(model.context, model.key, model.dataMap);

	return e;
}*/
