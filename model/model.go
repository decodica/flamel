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
	/*search.FieldLoadSaver
	searchQuery string
	searchable bool*/
	//represents the mapping of the modelable containing this Model
	*structure `model:"-"`

	key *datastore.Key `model:"-"`
	//the embedding modelable
	modelable modelable `model:"-"`
}

type structure struct {
	//encoded struct represents the mapping of the struct
	*encodedStruct
	//references point
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

func (model *Model) Save() ([]datastore.Property, error) {
	return toPropertyList(model.modelable);
}

func (model *Model) Load(props []datastore.Property) error {
	return fromPropertyList(model.modelable, props);
}

//returns true if the model has stale references
func (model *Model) hasStaleReferences() bool {
	m := model.modelable;
	mv := reflect.Indirect(reflect.ValueOf(m));

	for k, _ := range model.references {
		field := mv.Field(k);
		if field.Interface() != m {
			t := reflect.TypeOf(m).Elem().Field(k);
			log.Printf("Modelable %+v at address %p has stale references for field of type %s", m, &m, t.Name);
			return true;
		}
	}
	return false;
}

//Indexing maps the modelable to a linked-list-like structure.
//The indexing operation finds the modelable references and creates a map of them.
//Indexing keeps the keys in case of a reindex
//Indexing should not overwrite a model if it already exists.
//This method is called often, even for recursive operations.
//It is important to benchmark and optimize this code in order to not degrade performances
//of reads and writes calls to the Datastore.

//todo: benchmark and profile
func index(m modelable) {

	mType := reflect.TypeOf(m).Elem();
	//retrieve modelable anagraphics
	obj := reflect.ValueOf(m).Elem();
	name := mType.Name();

	var s structure;
	//check if the modelable structure has been already mapped
	model := Model{structure: &s};
	model.modelable = m;
	model.Registered = true;
	model.key = m.getModel().key;

	if enStruct, ok := encodedStructs[mType]; ok {
		s.encodedStruct = enStruct;
	} else {
		//map the struct
		s.encodedStruct = newEncodedStruct();
		mapStructure(mType, s.encodedStruct, name);
	}

	s.structName = name;

	//log.Printf("Modelable struct has name %s", s.structName);
	s.references = make(map[int]modelable);
	//map the references of the model
	for i := 0; i < obj.NumField(); i++ {

		fType := mType.Field(i);

		if obj.Field(i).Type() == typeOfModel {
			//skip mapping of the model
			continue
		}

		tags := strings.Split(fType.Tag.Get(tag_domain), ",");
		tagName := tags[0]

		if tagName == tag_skip {
			log.Printf("Field %s is skippable.", fType.Name);
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

				index(reference);
				//here the reference is registered
				s.references[i] = reference;
			}
		}
	}

	m.setModel(model);

	gob.Register(model.modelable);
}

//Returns true if the modelable is zero.
//This gets called many times in loops.
//todo: benchmark and profile
func isZero(ref modelable) bool {
	model := ref.getModel();

	v := reflect.Indirect(reflect.ValueOf(ref));
	t := reflect.TypeOf(ref).Elem();
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i);
		//ft := t.Field(i);

		//avoid checking models
		if field.Type() == typeOfModel {
			continue;
		}

		//if at least one field is valid we break the loop and we return false
		//it can be that model.references[i] is nil because the struct has not been registered
		if _, isRef := model.references[i];!isRef {
			if !isZeroOfType(field.Interface()) {
				return false;
			}
		}
	}

	log.Printf("\n !!!!Zero found for struct %s \n", t.Name());

	return true;
}

//Returns true if i is a zero value for its type
func isZeroOfType(i interface{}) bool {
	return i == reflect.Zero(reflect.TypeOf(i)).Interface();
}

//creates a datastore entity anmd stores the key into the model field
func create(ctx context.Context, m modelable) error {
	model := m.getModel();

	if (model.key != nil) {
		return errors.New("data has already been created");
	}

	for _, v := range model.references {
		rm := v.getModel();

		//todo: check if this is needed
		if rm.hasStaleReferences() {
			index(v);
		}

		if isZero(v) {
			log.Println("Skipped zero modelable in create");
			continue;
		}

		if rm.key == nil {
			err := create(ctx, v);
			if err != nil {
				return err;
			}
			continue
		}

		if rm.key != nil {
			err := update(ctx, v);
			if err != nil {
				return err;
			}
			continue;
		}
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

func read(ctx context.Context, m modelable) error {
	model := m.getModel();

	if model.hasStaleReferences() {
		index(m);
	}

	if model.key == nil {
		return errors.New(fmt.Sprintf("Can't populate struct %s. Model has no key", reflect.TypeOf(m).Elem().Name()));
	}

	err := datastore.Get(ctx, model.key, m);

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

func update(ctx context.Context, m modelable) error {
	model := m.getModel();

	if model.key == nil {
		return fmt.Errorf("Can't update modelable %v. Missing key", m);
	}

	//if the model is a zero we remove all its references
	/*if isZero(m) {
		return del(ctx, m);
	}*/

	log.Printf("\n\n Checking references for modelable %+v", m);
	for _, ref := range model.references {
		rm := ref.getModel();
		name := reflect.Indirect(reflect.ValueOf(ref)).Type().Name();

		//if the reference is zero, we delete it
		if isZero(ref) {
			err := del(ctx, ref);
			log.Printf("Struct %s is a zero. Deleting\n", name)
			if err != nil {
				return err;
			}
			continue;
		}

		//if the reference is stale, we reindex it
		if rm.hasStaleReferences() {
			index(m);
			log.Printf("Reindexed struct %s It is now %v \n", name, ref);
		}

		//if the reference doesn't have a key we create it
		if rm.key == nil {
			log.Printf("Struct %s has no key. Creating\n", name)
			err := create(ctx, ref);
			if err != nil {
				return err;
			}
			continue
		}

		//if the reference has a key we update it
		if rm.key != nil {
			err := update(ctx, ref);
			log.Printf("Struct %s has key. Updating\n", name)
			if err != nil {
				return err;
			}
			continue
		}
	}

	key, err := datastore.Put(ctx, model.key, m);

	if err != nil {
		return err;
	}

	model.key = key;

	return nil;
}

func del(ctx context.Context, m modelable) (err error) {
	model := m.getModel();

	if model.key == nil {
		//in this case, since we first loaded the modelable, we never inserted anything
		//we can skip the delete
		return errors.New("Can't delete struct %s. The key is nil");
	}

	for k, _ := range model.references {
		ref := model.references[k];
		err = del(ctx, ref);
		if err != nil {
			return err;
		}
	}
	return nil

	err = datastore.Delete(ctx, model.key);

	return err;
}

//Reads data from a modelable and writes it to the datastore as an entity with a new key.
func Create(ctx context.Context, m modelable) (err error) {
	model := m.getModel();

	if !model.Registered {
		index(m);
	} else if model.hasStaleReferences() {
		index(m);
	}

	defer func() {
		if err == nil {
			err = saveInMemcache(ctx, m);
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
	model := m.getModel();


	if !model.Registered {
		index(m);
	//use elseif so we avoid checking for stale refs since the model has been registered one line above
	} else if model.hasStaleReferences() {
		index(m);
	}

	log.Printf("Update called for model %+v\n\n\n", m);

	defer func() {
		if err == nil {
			err = saveInMemcache(ctx, m);
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
		index(m);
	}
	model.key = datastore.NewKey(ctx, model.structName, "", id, nil);
	return Read(ctx, m);
}

//Reads data from the datastore and writes them into the modelable.
//Writing into a modelable can happen only if the modelable is registered and has an ID.
func Read(ctx context.Context, m modelable) (err error) {
	if !m.getModel().Registered {
		index(m);
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
		//we first load the model
		err := read(ctx, m);
		if err == nil {
			return del(ctx, m);
		}
		return err;
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
