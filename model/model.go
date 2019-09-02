package model

import (
	"cloud.google.com/go/datastore"
	"context"
	"fmt"
	"reflect"
	"strings"
)

const valSeparator string = "."

const tagDomain string = "model"
const tagNoindex string = "noindex"
const tagZero string = "zero"
const tagAncestor string = "ancestor"

// Indicates that the given reference is "readonly"
// That is, it is provided from outside of the model
// An example would be the product model on a purchase model:
// Everytime a purchase is created or updated, the underlying product should not be written
// Beside improved performance obtained by avoiding writes on immutable model
// this prevents datastore contention if adequately used
const tagReadonly string = "readonly"
const tagSkip string = "-"

type modelable interface {
	getModel() *Model
	setModel(m Model)
}

//represents a child struct modelable.
//reference.Key and Modelable.getModel().Key might differ
type reference struct {
	// parent's index of reference field
	idx       int
	Modelable modelable
	Key       *datastore.Key
	Ancestor  bool
}

type structure struct {
	//encoded struct represents the mapping of the struct
	*encodedStruct
}

type Model struct {
	//Note: this is necessary to allow simple implementation of memcache encoding and coding
	//else we get the all unexported fields error from Gob package
	registered bool `model:"-"`

	//represents the mapping of the modelable containing this Model
	*structure `model:"-"`

	references []reference `model:"-"`

	Key *datastore.Key `model:"-"`
	//the embedding modelable
	modelable modelable `model:"-"`
}

func (model *Model) getModel() *Model {
	return model
}

func (model *Model) setModel(m Model) {
	*model = m
}

func (model Model) referenceAtIndex(idx int) *reference {
	// for small number of elements, such as references of a struct, a linear search is quick enough
	for _, v := range model.references {
		if v.idx == idx {
			return &v
		}
	}
	return nil
}

func (model Model) extensionNameAtIndex(idx int) string {
	inmodel := model.encodedStruct.extensionsIdx[idx]
	return reflect.TypeOf(model.modelable).Field(inmodel).Name
}

func IsEmpty(m modelable) bool {
	model := m.getModel()
	if !model.isRegistered() {
		index(m)
	}
	return model.Key == nil && isZero(model.modelable)
}

// recursively checks if the references of the model have been registered.
// todo: re-structure the model to increase performance
func (model Model) isRegistered() bool {
	if !model.registered {
		return false
	}

	for _, ref := range model.references {
		rm := ref.Modelable.getModel()
		if !rm.isRegistered() {
			return false
		}
	}

	return true
}

//Loads values from the datastore for the entity with the given id.
//Entity types must be the same with m and the entity whose id is id
func FromIntID(ctx context.Context, m modelable, id int64, ancestor modelable) error {
	model := m.getModel()
	if !model.isRegistered() {
		index(m)
	}

	var ancKey *datastore.Key = nil

	if ancestor != nil {
		if ancestor.getModel().Key == nil {
			return fmt.Errorf("ancestor %v has no Key", ancestor)
		}
		ancKey = ancestor.getModel().Key
	}

	model.Key = datastore.IDKey(model.structName, id, ancKey)
	return Read(ctx, m)
}

//Loads values from the datastore for the entity with the given string id.
//Entity types must be the same with m and the entity whos id is id
func FromStringID(ctx context.Context, m modelable, id string, ancestor modelable) error {
	model := m.getModel()
	if !model.isRegistered() {
		index(m)
	}

	var ancKey *datastore.Key = nil

	if ancestor != nil {
		if ancestor.getModel().Key == nil {
			return fmt.Errorf("ancestor %v has no Key", ancestor)
		}
		ancKey = ancestor.getModel().Key
	}

	model.Key = datastore.NameKey(model.structName, id, ancKey)
	return Read(ctx, m)
}

func FromEncodedKey(ctx context.Context, m modelable, skey string) error {
	model := m.getModel()

	key, err := datastore.DecodeKey(skey)

	if err != nil {
		return err
	}

	model.Key = key

	return Read(ctx, m)
}

//returns -1 if the model doesn't have an id
//returns the id of the model otherwise
func (model Model) IntID() int64 {
	if model.Key == nil {
		return -1
	}

	return model.Key.ID
}

func (model Model) StringID() string {
	if model.Key == nil {
		return ""
	}
	return model.Key.Name
}

//Returns the name of the modelable this model refers to
func (model Model) Name() string {
	return model.structName
}

func (model Model) EncodedKey() string {
	if model.Key == nil {
		return ""
	}

	return model.Key.Encode()
}

func (model *Model) Save() ([]datastore.Property, error) {
	return toPropertyList(model.modelable)
}

func (model *Model) Load(props []datastore.Property) error {
	return fromPropertyList(model.modelable, props)
}

// Indexing maps the modelable to a linked-list-like structure.
// The indexing operation finds the modelable references and stores them into an instance map.
// Indexing keeps the keys in case of a reindex
// Indexing should not overwrite a model if it already exists.
// This method is called often, even for recursive operations.
// It is important to benchmark and optimize this code in order to not degrade performances
// of reads and writes calls to the Datastore.

func index(m modelable) {
	mType := reflect.TypeOf(m).Elem()
	obj := reflect.ValueOf(m).Elem()
	//retrieve modelable anagraphics
	name := mType.Name()

	model := m.getModel()
	key := model.Key

	//check if the modelable structure has been already mapped
	if model.structure == nil {
		model.structure = &structure{}
	}

	//set the model to point to the new modelable
	//in case it was previously pointing to the old one
	model.modelable = m
	model.registered = true
	model.Key = key

	//we assign the structure to the model.
	//if we already mapped the same struct earlier we get it from the cache
	if enStruct, ok := encodedStructs[mType]; ok {
		model.structure.encodedStruct = enStruct
	} else {
		//we didn't map the structure earlier on. Map it now
		model.structure.encodedStruct = newEncodedStruct(name)
		mapStructure(mType, model.structure.encodedStruct)
	}

	hasAncestor := false

	// register model extensions
	for _, idx := range model.encodedStruct.extensionsIdx {
		ef := obj.Field(idx)
		if ef.IsNil() {
			continue
		}

		et := ef.Elem().Type().Elem()
		if _, ok := encodedStructs[et]; !ok {
			mapStructure(et, newEncodedStruct(et.Name()))
		}
	}

	if model.references == nil {

		refno := len(model.encodedStruct.referencesIdx)
		//if we have no references mapped we rebuild the mapping
		model.references = make([]reference, refno)

		for idx, num := range model.encodedStruct.referencesIdx {
			fType := mType.Field(num)
			tags := strings.Split(fType.Tag.Get(tagDomain), ",")

			isAnc := containsTag(tags, tagAncestor) != ""

			if isAnc {
				//flag the index as the ancestor
				//if already has an ancestor we throw an error
				if hasAncestor {
					err := fmt.Errorf("multiple ancestors set for model of type %s", mType.Name())
					panic(err)
				}
				hasAncestor = true
			}

			rm := obj.Field(num).Addr().Interface().(modelable)

			index(rm)
			//here the reference is registered
			//if we already have the reference we update the modelable
			hr := reference{}
			// set the field idx of the reference
			hr.idx = num
			hr.Modelable = rm
			hr.Ancestor = isAnc
			model.references[idx] = hr
		}

		//if we already have references we update the modelable they point to
	} else {
		for i, ref := range model.references {

			// register the reference if not registered
			// this can happen if a reference allows to be zeroed and the parent model has been read
			// from the datastore
			if !ref.Modelable.getModel().isRegistered() {
				index(ref.Modelable)
				continue
			}

			// if the reference has been changed since our last check, we must register the new reference
			// to replace the stale one.
			orig := ref.Modelable
			newRef := obj.Field(ref.idx).Addr().Interface().(modelable)

			if orig == newRef {
				continue
			}

			om := orig.getModel()

			nm := newRef.getModel()
			nm.modelable = newRef
			nm.references = om.references
			nm.structure = om.structure
			nm.structName = om.structName
			newRef.setModel(*nm)

			index(newRef)

			ref.Modelable = newRef
			model.references[i] = ref
		}
	}

	m.setModel(*model)
}

// Returns a pointer to the Model the container is holding
func modelOf(src interface{}) *Model {
	m, ok := src.(modelable)
	if ok {
		return m.getModel()
	}

	//if src is not a modelable we check if it is a slice of modelables
	dstv := reflect.ValueOf(src)

	var val reflect.Value

	if dstv.Kind() == reflect.Ptr {
		val = dstv.Elem()
		if val.Kind() == reflect.Slice {
			typ := val.Type().Elem()
			val = reflect.New(typ.Elem())
		} else if val.Kind() == reflect.Struct {
			return modelOf(val)
		} else {
			return nil
		}
	} else if dstv.Kind() == reflect.Slice {
		typ := reflect.TypeOf(src).Elem()
		val = reflect.New(typ.Elem())
	} else {
		// not a container and not a modelable, return nil
		return nil
	}

	m, ok = val.Interface().(modelable)
	if ok {
		index(m)
		return m.getModel()
	}

	return nil
}
