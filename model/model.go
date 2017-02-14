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

	references map[int]reference `model:"-"`

	key *datastore.Key `model:"-"`
	//the embedding modelable
	modelable modelable `model:"-"`
}

//represents a child struct modelable.
//reference.Key and Modelable.getModel().Key might differ
type reference struct {
	Modelable modelable
	Key *datastore.Key
}

type structure struct {
	//encoded struct represents the mapping of the struct
	*encodedStruct
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


func (reference *reference) isStale() bool {
	return reference.Modelable.getModel().key != reference.Key;
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

	model := m.getModel();
	key := model.key;

	//check if the modelable structure has been already mapped
	if model.structure == nil {
		model.structure = &structure{};
	}

	model.modelable = m;
	model.Registered = true;
	model.key = key;

	//we assign the structure to the model.
	//if we already mapped the same struct earlier we get it from the cache
	if enStruct, ok := encodedStructs[mType]; ok {
		model.structure.encodedStruct = enStruct;
	} else {
		//map the struct
		model.structure.encodedStruct = newEncodedStruct();
		mapStructure(mType, model.structure.encodedStruct, name);
	}

	model.structure.structName = name;

	if model.references == nil {
		//if we have no references mapped we rebuild the mapping
		model.references = make(map[int]reference);

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

				if rm, ok := obj.Field(i).Addr().Interface().(modelable); ok {
					//we register the modelable

					index(rm);
					//here the reference is registered
					//if we already have the reference we update the modelable
					hr := reference{};
					hr.Modelable = rm;
					model.references[i] = hr;

				}
			}
		}
	//if we already have references we update the modelable they point to
	} else {

		for k, _ := range model.references {
			ref := model.references[k];
			//we get the old reference
			orig := ref.Modelable;
			//we get the new reference
			newRef := obj.Field(k).Addr().Interface().(modelable);

			if orig == newRef {
				continue
			}

			om := orig.getModel();

			nm := newRef.getModel();
			nm.modelable = newRef;
			nm.references = om.references;
			nm.structure = om.structure;
			nm.structName = om.structName;
			newRef.setModel(*nm);

			index(newRef)

			ref.Modelable = newRef;
			model.references[k] = ref;
		}
	}

	m.setModel(*model);

	gob.Register(model.modelable);
}

//Returns true if the modelable is zero.
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

//creates a datastore entity and stores the key into the model field
func create(ctx context.Context, m modelable) error {
	model := m.getModel();

	//if the root model has a key then this is the wrong operation
	if (model.key != nil) {
		return errors.New("data has already been created");
	}

	//we iterate through the model references.
	//if a reference has its own key we use it as a value in the root entity
	for k, _ := range model.references {
		ref := model.references[k];
		rm := ref.Modelable.getModel();

		if ref.Key != nil {
			//this can't happen because we are in create, thus the root model can't have a key
			//and can't have its reference's key populated
			return errors.New("Create called with a non-nil reference map");
		} else {

			//case is that the reference has been loaded from the datastore
			//we update the reference values using the reference key
			//then we update the root reference map key
			if rm.key != nil {
				err := updateReference(ctx, &ref, rm.key);
				if err != nil {
					return err;
				}
			} else {
				err := createReference(ctx, &ref);
				if err != nil {
					return err;
				}
			}
		}

		model.references[k] = ref;
	}

	incompleteKey := datastore.NewIncompleteKey(ctx, model.structName, nil);

	key, err := datastore.Put(ctx, incompleteKey, m);

	if err != nil {
		return err;
	}

	model.key = key;

	return nil;
}



func updateReference(ctx context.Context, ref *reference, key *datastore.Key) (err error) {

	model := ref.Modelable.getModel();

	log.Printf("\n\nUpdating references for modelable %+v. ", ref.Modelable)
	log.Printf("\n\nParent has keys %+v. ", model.references)

	//we iterate through the references of the current model
	for k, _ := range model.references {
		r := model.references[k];
		rm := r.Modelable.getModel();
		//We check if the parent has a key related to the reference.
		//If it does we use the key provided by the parent to update the childrend
		if r.Key != nil {
			log.Printf("\n\nModel.references.key not nil for %v. Updating using parent refkey", r.Modelable)
			err := updateReference(ctx, &r, r.Key);
			if err != nil {
				return err;
			}
		} else {
			//else, if the parent doesn't have the key we must check the children
			if rm.key != nil {
				log.Printf("\n\nChild.Model.key not nil for %v. Updating using child key", r.Modelable)
				//the child was loaded and then assigned to the parent: update the children
				//and make the parent point to it
				err := updateReference(ctx, &r, rm.key);
				if err != nil {
					return err;
				}
			} else {
				//neither the parent and the children specify a key.
				//We create the children and update the parent's key
				log.Printf("\n\nNo key available for %+v. Creating entity", r.Modelable)
				err := createReference(ctx, &r);
				if err != nil {
					return err;
				}
			}
		}

		model.references[k] = r;
	}

	//we align ref and parent key
	model.key = key;
	ref.Key = key;

	_, err = datastore.Put(ctx, key, ref.Modelable);

	if err != nil {
		return err;
	}

	return nil;
}


//creates a reference
func createReference(ctx context.Context, ref *reference) (err error) {

	err = create(ctx, ref.Modelable);

	if err != nil {
		return err;
	}

	defer func() {
		if err != nil {
			ref.Key = ref.Modelable.getModel().key;
		}
	}();

	return nil;
}


func read(ctx context.Context, m modelable) error {
	model := m.getModel();

	if model.key == nil {
		return errors.New(fmt.Sprintf("Can't populate struct %s. Model has no key", reflect.TypeOf(m).Elem().Name()));
	}

	err := datastore.Get(ctx, model.key, m);

	if err != nil {
		return err;
	}

	for k, _ := range model.references {
		ref := model.references[k];
		rm := ref.Modelable.getModel();

		err := read(ctx, ref.Modelable);
		if err != nil {
			return err;
		}

		ref.Key = rm.key;
		model.references[k] = ref;

		log.Printf("\n Read reference for model which now has references %+v", model.references)
	}

	return nil
}

//updates the given modelable
//iterates through the modelable reference.
//if the reference has a key
func update(ctx context.Context, m modelable) error {
	model := m.getModel();

	if model.key == nil {
		return fmt.Errorf("Can't update modelable %v. Missing key", m);
	}

	for k, _ := range model.references {
		ref := model.references[k];
		rm := ref.Modelable.getModel();

		if rm.key != nil {
			log.Printf("\n Updating reference with own key. using key %s", rm.key.Encode())
			err := updateReference(ctx, &ref, rm.key);
			if err != nil {
				return err;
			}
		} else {
			log.Printf("\n Updating reference with parent key. using key %s", ref.Key.Encode())
			err := updateReference(ctx, &ref, ref.Key);
			if err != nil {
				return err;
			}
		}

		model.references[k] = ref;
	}

	key, err := datastore.Put(ctx, model.key, m);

	if err != nil {
		return err;
	}

	model.key = key;

	return nil;
}

/*func del(ctx context.Context, m modelable) (err error) {
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
}*/

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
//If a reference is read from the storage and then assigned to the root modelable
//the root modelable will point to the loaded entity
//If a reference is newly created its value will be updated accordingly to the model
func Update(ctx context.Context, m modelable) (err error) {
	model := m.getModel();
	log.Printf("Update called for modelable %+v. \n\n Model has keys %+v \n\n\n", m, model.references);


	if !model.Registered {
		index(m);
	//use elseif so we avoid checking for stale refs since the model has been registered one line above
	} else if model.hasStaleReferences() {
		index(m);
	}

	log.Printf("\n\nAfter indexing, modelable is %+v and has keys %+v \n\n\n", m, model.references);

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

	/*err = loadFromMemcache(ctx, m);

	if err == nil {
		return err
	}*/

	err = datastore.RunInTransaction(ctx, func (ctx context.Context) error {
		return read(ctx, m);
	}, &opts)

	return err;
}

/*func Delete(ctx context.Context, m modelable) (err error) {

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
}*/

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
