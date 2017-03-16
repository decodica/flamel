package model

import (
	"google.golang.org/appengine/datastore"
	gaelog "google.golang.org/appengine/log"
	"reflect"
	"golang.org/x/net/context"
	"fmt"
	"errors"
	"strings"
	"encoding/gob"
	"google.golang.org/appengine/memcache"
)

const ref_model_prefix string = "ref_";

const model_modelable_field_name string = "modelable";

const val_serparator string = ".";

const tag_domain string = "model";

const default_entry_count_per_read_batch int = 500;



const tag_skip string = "-";
const tag_search string = "search";
const tag_noindex string = "noindex";
const tag_ancestor string = "ancestor";

type modelable interface {
	getModel() *Model
	setModel(m Model)
}

type Model struct {
	//Note: this is necessary to allow simple implementation of memcache encoding and coding
	//else we get the all unexported fields error from Gob package
	registered bool `model:"-"`
	/*search.FieldLoadSaver
	searchQuery string
	searchable bool*/
	//represents the mapping of the modelable containing this Model
	*structure `model:"-"`

	references map[int]reference `model:"-"`

	Key *datastore.Key `model:"-"`
	//the embedding modelable
	modelable modelable `model:"-"`
}

//represents a child struct modelable.
//reference.Key and Modelable.getModel().Key might differ
type reference struct {
	Modelable modelable
	Key *datastore.Key
	Ancestor bool
}

type structure struct {
	//encoded struct represents the mapping of the struct
	*encodedStruct
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
func (model Model) IntID() int64 {
	if model.Key == nil {
		return -1;
	}

	return model.Key.IntID();
}

func (model Model) StringID() string {
	if model.Key == nil {
		return ""
	}
	return model.Key.StringID()
}

//Returns the name of the modelable this model refers to
func (model Model) Name() string {
	return model.structName;
}

func (model Model) EncodedKey() string {
	if model.Key == nil {
		return "";
	}

	return model.Key.Encode();
}

func (model *Model) Save() ([]datastore.Property, error) {
	return toPropertyList(model.modelable);
}

func (model *Model) Load(props []datastore.Property) error {
	return fromPropertyList(model.modelable, props);
}

//returns true if the model has stale references
//todo: control validity - this may be incorrect with equality
func (model *Model) hasStaleReferences() bool {
	m := model.modelable;

	mv := reflect.Indirect(reflect.ValueOf(m));

	for k, _ := range model.references {
		field := mv.Field(k);
		ref := model.references[k];
		if field.Interface() != ref.Modelable {
			return true;
		}
	}
	return false;
}


func (reference *reference) isStale() bool {
	return reference.Modelable.getModel().Key != reference.Key;
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
	obj := reflect.ValueOf(m).Elem();
	//retrieve modelable anagraphics

	name := mType.Name();

	model := m.getModel();
	Key := model.Key;

	//check if the modelable structure has been already mapped
	if model.structure == nil {
		model.structure = &structure{};
	}

	//set the model to point to the new modelable
	//in case it was previously pointing to the old one
	model.modelable = m;
	model.registered = true;
	model.Key = Key;

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

	hasAncestor := false;

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
					isAnc := tagName == tag_ancestor;

					if isAnc {
						//flag the index as the ancestor
						//if already has an ancestor we throw an error
						if hasAncestor {
							err := fmt.Errorf("Multiple ancestors set for model of type %s", mType.Name());
							panic(err);
						}
						hasAncestor = true;
					}

					index(rm);
					//here the reference is registered
					//if we already have the reference we update the modelable

					hr := reference{};
					hr.Modelable = rm;
					hr.Ancestor = isAnc;
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

	return true;
}

//Returns true if i is a zero value for its type
func isZeroOfType(i interface{}) bool {
	return i == reflect.Zero(reflect.TypeOf(i)).Interface();
}

func createWithOptions(ctx context.Context, m modelable, opts *CreateOptions) error {
	model := m.getModel();

	//if the root model has a Key then this is the wrong operation
	if (model.Key != nil) {
		return errors.New("data has already been created");
	}

	var ancKey *datastore.Key = nil;
	//we iterate through the model references.
	//if a reference has its own Key we use it as a value in the root entity
	for k, _ := range model.references {
		ref := model.references[k];
		rm := ref.Modelable.getModel();
		if ref.Key != nil {
			//this can't happen because we are in create, thus the root model can't have a Key
			//and can't have its reference's Key populated
			return errors.New("Create called with a non-nil reference map");
		} else {
			//case is that the reference has been loaded from the datastore
			//we update the reference values using the reference Key
			//then we update the root reference map Key
			if rm.Key != nil {
				err := updateReference(ctx, &ref, rm.Key);
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
		if ref.Ancestor {
			ancKey = ref.Key;
		}
		model.references[k] = ref;
	}

	//newKey := datastore.NewIncompleteKey(ctx, model.structName, ancKey);
	newKey := datastore.NewKey(ctx, model.structName, opts.stringId, opts.intId, ancKey);
	Key, err := datastore.Put(ctx, newKey, m);
	if err != nil {
		return err;
	}
	model.Key = Key;

	return nil;
}

//creates a datastore entity and stores the Key into the model field
func create(ctx context.Context, m modelable) error {
	opts := NewCreateOptions();
	return createWithOptions(ctx, m, &opts);
}

func updateReference(ctx context.Context, ref *reference, Key *datastore.Key) (err error) {
	model := ref.Modelable.getModel();

	//we iterate through the references of the current model
	for k, _ := range model.references {
		r := model.references[k];
		rm := r.Modelable.getModel();
		//We check if the parent has a Key related to the reference.
		//If it does we use the Key provided by the parent to update the childrend
		if r.Key != nil {
			err := updateReference(ctx, &r, r.Key);
			if err != nil {
				return err;
			}
		} else {
			//else, if the parent doesn't have the Key we must check the children
			if rm.Key != nil {
				//the child was loaded and then assigned to the parent: update the children
				//and make the parent point to it
				err := updateReference(ctx, &r, rm.Key);
				if err != nil {
					return err;
				}
			} else {
				//neither the parent and the children specify a Key.
				//We create the children and update the parent's Key
				err := createReference(ctx, &r);
				if err != nil {
					return err;
				}
			}
		}

		model.references[k] = r;
	}

	//we align ref and parent Key
	model.Key = Key;
	ref.Key = Key;

	_, err = datastore.Put(ctx, Key, ref.Modelable);

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
		if err == nil {
			ref.Key = ref.Modelable.getModel().Key;
		}
	}();

	return nil;
}


func read(ctx context.Context, m modelable) error {
	model := m.getModel();

	if model.Key == nil {
		return errors.New(fmt.Sprintf("Can't populate struct %s. Model has no Key", reflect.TypeOf(m).Elem().Name()));
	}

	err := datastore.Get(ctx, model.Key, m);

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
		ref.Key = rm.Key;
		model.references[k] = ref;
	}

	return nil
}

//updates the given modelable
//iterates through the modelable reference.
//if the reference has a Key
func update(ctx context.Context, m modelable) error {
	model := m.getModel();

	if model.Key == nil {
		return fmt.Errorf("Can't update modelable %v. Missing Key", m);
	}

	for k, _ := range model.references {
		ref := model.references[k];
		rm := ref.Modelable.getModel();

		if rm.Key != nil {
			err := updateReference(ctx, &ref, rm.Key);
			if err != nil {
				return err;
			}
		} else {
			err := updateReference(ctx, &ref, ref.Key);
			if err != nil {
				return err;
			}
		}

		model.references[k] = ref;
	}

	Key, err := datastore.Put(ctx, model.Key, m);

	if err != nil {
		return err;
	}

	model.Key = Key;

	return nil;
}

func del(ctx context.Context, m modelable) (err error) {
	model := m.getModel();

	if model.Key == nil {
		return errors.New("Can't delete struct . The Key is nil");
	}

	for k, _ := range model.references {
		ref := model.references[k];
		err = del(ctx, ref.Modelable);
		if err != nil {
			return err;
		}
	}

	err = datastore.Delete(ctx, model.Key);

	return err;
}

type CreateOptions struct {
	stringId string
	intId int64
}

func NewCreateOptions() CreateOptions {
	return CreateOptions{};
}

func (opts *CreateOptions) WithStringId(id string) {
	opts.intId = 0;
	opts.stringId = id;
}

func (opts *CreateOptions) WithIntId(id int64) {
	opts.stringId = "";
	opts.intId = id;
}

func CreateWithOptions(ctx context.Context, m modelable, copts *CreateOptions) (err error) {
	model := m.getModel();

	if !model.registered {
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
		return createWithOptions(ctx, m, copts);
	}, &opts)

	return err;
}

//Reads data from a modelable and writes it to the datastore as an entity with a new Key.
func Create(ctx context.Context, m modelable) (err error) {
	return CreateWithOptions(ctx, m, new(CreateOptions));
}

//Reads data from a modelable and writes it into the corresponding entity of the datastore.
//If a reference is read from the storage and then assigned to the root modelable
//the root modelable will point to the loaded entity
//If a reference is newly created its value will be updated accordingly to the model
func Update(ctx context.Context, m modelable) (err error) {
	model := m.getModel();
	if !model.registered {
		index(m);
	//use elseif so we avoid checking for stale refs since the model has been registered one line above
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
		return update(ctx, m);
	}, &opts)

	return err
}

//Loads values from the datastore for the entity with the given id.
//Entity types must be the same with m and the entity whose id is id
func FromIntID(ctx context.Context, m modelable, id int64, ancestor modelable) error {
	model := m.getModel();
	if !model.registered {
		index(m);
	}

	var ancKey *datastore.Key = nil;

	if ancestor != nil {
		if ancestor.getModel().Key == nil {
			return fmt.Errorf("Ancestor %v has no Key", ancestor);
		}
		ancKey = ancestor.getModel().Key;
	}

	model.Key = datastore.NewKey(ctx, model.structName, "", id, ancKey);
	return Read(ctx, m);
}

//Loads values from the datastore for the entity with the given string id.
//Entity types must be the same with m and the entity whos id is id
func FromStringID(ctx context.Context, m modelable, id string, ancestor modelable) error {
	model := m.getModel();
	if !model.registered {
		index(m);
	}

	var ancKey *datastore.Key = nil;

	if ancestor != nil {
		if ancestor.getModel().Key == nil {
			return fmt.Errorf("Ancestor %v has no Key", ancestor);
		}
		ancKey = ancestor.getModel().Key;
	}

	model.Key = datastore.NewKey(ctx, model.structName, id, 0, ancKey);
	return Read(ctx, m);
}

func FromEncodedKey(ctx context.Context, m modelable, skey string) error {
	model := m.getModel();

	key, err := datastore.DecodeKey(skey);

	if err != nil {
		return err;
	}

	model.Key = key;

	return Read(ctx, m);
}

//Reads data from the datastore and writes them into the modelable.
func Read(ctx context.Context, m modelable) (err error) {
	model := m.getModel();
	if !model.registered {
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
			if err != nil && err != memcache.ErrCacheMiss{
				gaelog.Errorf(ctx, "Error deleting items from memcache: %v", err);
			}
		}
	}();

	opts := datastore.TransactionOptions{}
	opts.Attempts = 1;
	opts.XG = true;

	err = datastore.RunInTransaction(ctx, func (ctx context.Context) error {
		return del(ctx, m);
	}, &opts);

	return err;
}