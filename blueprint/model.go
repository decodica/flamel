package blueprint

import (
	"google.golang.org/appengine/datastore"
//	"errors"
	"google.golang.org/appengine"
	"reflect"
	"time"
//	"fmt"
//	"google.golang.org/appengine/memcache"
//	"encoding/gob"
//	"google.golang.org/appengine/search"
//	"strings"
	"golang.org/x/net/context"
//	"google.golang.org/appengine/log"
//	"sync"
	"log"
	"fmt"
	"errors"
	"strings"
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
	//*dataMap
	/*search.FieldLoadSaver
	searchQuery string
	searchable bool*/
	//represents the mapping of the modelable containing this Model
	*structure
	//tool used in keeping trace of nested structs. todo: empty once loading is over
	//it maps field with field position and keeps the record
	propertyLoader `model:"-"`

	key *datastore.Key

	//the embedding modelable
	modelable modelable `model:"-"`

}

type structure struct {
	//encoded struct represents the mapping of the struct
	*encodedStruct
	references map[int]modelable
}

type dataMap struct {
	context    context.Context

	entityName string
	m          Prototype
	//references maps the field index with the prototype struct
	references map[int]*Model
	//values maps field indices with structs values
	values     map[string]encodedField
	datastore.PropertyLoadSaver
	//datastore query state
	query      *datastore.Query
//	propertyLoader
	Debug bool
	loadRef bool
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

var encodedStructs = map[reflect.Type]*encodedStruct{}

//Model satisfies modelable
//Returns the current Model.
func (model *Model) getModel() *Model {
	return model;
}

//Set the value of m into the Model
func (model *Model) setModel(m Model) {
	*model = m;
}


//Records the modelable structure into the modelable Model object.
//See the Gob package analogous method
func Register(m modelable) error {

	mType := reflect.TypeOf(m).Elem()
	//retrieve modelable anagraphics
	obj := reflect.ValueOf(m).Elem()
	name := obj.Type().Name()

	var s structure;
	//check if the modelable structure has been already mapped
	if enStruct, ok := encodedStructs[mType]; ok {
		s.encodedStruct = enStruct;
	} else {
		//map the struct
		s.encodedStruct = newEncodedStruct()
		s.structName = name;
		mapStructure(mType, s.encodedStruct, name)
	}

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
				return fmt.Errorf("Unaddressable reference %v in Model", obj.Field(i))
			}

			if reference, ok := obj.Field(i).Addr().Interface().(modelable); ok {
				//we register the modelable
				err := Register(reference)
				if nil != err {
					return err
				}
				s.references[i] = reference
			}
		}
	}

	model := Model{structure: &s}
	//sets the wrapping object to the model (????)
	model.modelable = m;

	m.setModel(model)
	log.Printf("Mapped modelable of type %s: %+v", mType, m)
	log.Printf("Fields are %+v", model.fieldNames)

	return nil
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

	//first create references and get the keys
	for k, _ := range model.references {
		ref := model.references[k];
		refModel := ref.getModel();
		log.Printf(">>>>> Creating reference %v ", ref)
		if refModel.key == nil {
			err := create(ctx, ref);
			if err != nil {
				log.Printf("Transaction for reference failed with error %s", err.Error())
				return err;
			}

		} else {
			//todo: update
		}
	}

	incompleteKey := datastore.NewIncompleteKey(ctx, model.structName, nil);
	log.Printf(">>>>> Incomplete for struct %s key is %s ", model.structName, incompleteKey.String())

	key, err := datastore.Put(ctx, incompleteKey, m);

	if (err != nil) {
		return err;
	}

	model.key = key;

	return nil;

	//if data is cached, create the item in the memcache
	//	data.Print(" ==== MEMCACHE ==== SET IN CREATE FOR data " + data.entityName);

//	data.cacheSet();


//	data.Print("data " + data.entityName + " successfully created");
}

func Create(ctx context.Context, m modelable) error {
	opts := datastore.TransactionOptions{}
	opts.XG = true;
	opts.Attempts = 1;
	return datastore.RunInTransaction(ctx, func (ctx context.Context) error {
		return create(ctx, m);
	}, &opts)
}

func populate(ctx context.Context, m modelable) error {
	/*model := m.getModel();

	if model.key == nil {
		return errors.New(fmt.Sprintf("Can't read struct %s. Model has no key", model.structName));
	}

	err := datastore.Get(ctx, model.key, model)

	if err != nil {
		return err;
	}

	for k, _ := range model.references {
		ref := model.references[k];
		populate(ctx, ref);
	}*/

	return nil
}



//Creates a new model
//A model is composed of the following properties:
//- references: prototypes are considered references. They are structs
// that implement the Modelable interface.
//references are saved as keys in the containing struct model and as different entities
//- values: they are values of the model. They can also be nested structs
//Values are saved as containing structs properties, with a Nested.Struct name
/*func NewModel(c context.Context, m Prototype) (*Model, error) {

	//get the prototype struct name
	name := nameOfPrototype(m);

	prototypeValue := reflect.ValueOf(m).Elem();

	references := make(map[int]*Model);
	values := make(map[string]encodedField);

	searchable := false;
	//traverse the struct fields and check if a field is a reference
	for i := 0; i < prototypeValue.NumField(); i++ {

		field := prototypeValue.Field(i);
		fieldType := prototypeValue.Type().Field(i);

		sName := fieldType.Name;
		valueField := encodedField{index:i};

		tagName, _ := fieldType.Tag.Get(tag_domain), "";
		if i := strings.Index(name, ","); i != -1 {
			tagName, _ = name[:i], name[i + 1:];
		}

		//log.Printf("NAME: " + tagName + ", TAGS: %v", tags);

		//if at least one field is tagged for search, flag the model as searchable

		if tagName == tag_search {
			searchable = true;
			gPrint("MODEL " + name + " IS SEARCHABLE");
		}

		switch field.Kind() {
		case reflect.Slice:

			//se slice di struct, prendi il tipo di struct ed encodalo
			if field.Type().Elem().Kind() != reflect.Struct {
				break;
			}

			elem := fieldType.Type.Elem();
			gPrintf("Slice proto: %+v", elem);
			//for now consider only value and not arrays of reference structs
			sMap := make(map[string]encodedField);

			childStruct := &encodedStruct{structName:sName, fieldNames:sMap}
			valueField.childStruct = childStruct;

			//encode the child struct
			mapValueStruct(elem, childStruct, sName);

		//if a field is a struct, check if it implements the modelable interface.
		//if it does, treat it as a reference
		case reflect.Struct:
			if !field.CanAddr() {
				return nil, fmt.Errorf("datastore: unsupported struct field: value is unaddressable")
			}

			//check if struct is of type time or value
			if fieldType.Type == typeOfTime || fieldType.Type == typeOfGeoPoint {
				break;
			}

			//if reference, use the value to create and assign a new model
			//don't map the struct, the ref model will handle that
			reference, ok := field.Addr().Interface().(Prototype);

			if ok {
				//log.gPrint("==== NEW MODEL==== Found a reference at index " + strconv.Itoa(i) + ": " + field.String());
				refModel, err := NewModel(c, reference);
				if nil != err {
					panic("Can't create reference model")
				}
				references[i] = refModel;
				//put the struct into the values array and flag it as a reference
				values[makeRefname(refModel.entityName)] = valueField;
				continue;
			}

			//else it is a value, take note of this sub-struct
			sMap := make(map[string]encodedField);

			//instantiate the encoded struct
			childStruct := &encodedStruct{structName:sName, fieldNames:sMap}

			valueField.childStruct = childStruct;

			//encode the child struct
			mapValueStruct(fieldType.Type, childStruct, sName);
			//add the mapped struct to the value ,map

			//log.gPrint("==== NEW MODEL ===== FOUND VALUE STRUCT AT INDEX " + strconv.Itoa(i) + " WITH NAME: " + sName);
			//log.gPrint(values);
			break;
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			fallthrough
		case reflect.Bool:
			fallthrough
		case reflect.String:
			fallthrough
		case reflect.Float32, reflect.Float64:
			fallthrough
		default:
			break;

		}
		values[sName] = valueField;

	}

	//log.gPrint("==== NEW MODEL==== BUILT MODEL FOR ENTITY " + name );
	//register type for memcache
	gob.Register(m);
	data := &dataMap{context:c, m:m, entityName:name, references:references, values:values, loadRef: true};
	return &Model{dataMap:data, searchable:searchable}, nil;
}


//TODO: CLONE MODEL TO KEEP OPTIONS ACTIVE BETWEEN POINTER SUBSTITUTION

func (model *Model) SetReference(ref Model) error {
	refName := makeRefname(ref.entityName);
	s, ok := model.values[refName];

	if !(ok && s.isReference) {
		return errors.New("Struct not found for name " + refName);
	}

	_, ok = model.references[s.index];

	if !ok {
		return errors.New("Reference with name " + refName + " not found.");
	}

	//if the reference is found and it is valid,
	//update the main struct values
	protoValue := reflect.ValueOf(model.Prototype()).Elem();
	refProto := ref.Prototype();
	//this works since only structs are supported as references
	//todo:support slices as well
	protoValue.Field(s.index).Set(reflect.ValueOf(refProto).Elem());

	model.references[s.index] = &ref;

	return nil;
}

func (model *Model) GetReference(name string) (*Model, error) {
	refName := makeRefname(name);
	s, ok := model.values[refName];

	if !(ok && s.isReference) {
		return nil, errors.New("Reference not found for name " + refName);
	}

	ref, ok := model.references[s.index];

	if !ok {
		return nil, errors.New("Reference not found for name " + refName);
	}

	return ref, nil;
}

func (data *dataMap) Save() ([]datastore.Property, error) {
	return data.toPropertyList();
}

func (data *dataMap) Load(props []datastore.Property) error {
	return data.fromPropertyList(props);
}

func nameOfModelable(m modelable) string {
	return reflect.ValueOf(m).Elem().Type().String();
}

func makeRefname(base string) string {
	return ref_model_prefix + base;
}

func (data *dataMap) create() error {
	if (data.key != nil) {
		return errors.New("data has already been created");
	}

	incompleteKey := datastore.NewIncompleteKey(data.context, data.entityName, nil);

	key, err := datastore.Put(data.context, incompleteKey, data);

	if (err != nil) {
		return err;
	}

	data.key = key;
	//if data is cached, create the item in the memcache
	data.Print(" ==== MEMCACHE ==== SET IN CREATE FOR data " + data.entityName);

	data.cacheSet();


	data.Print("data " + data.entityName + " successfully created");
	return nil;
}

func (model *Model) Create() error {
	var e error;
	if model.key != nil {
		return errors.New("Model has already been created");
	}

	e = model.dataMap.create();

	if nil == e && model.searchable {
		model.Index();
	}

	return e;
}


//todo: move reads from memcache/struct property load to this method. foreach ref, read it
func (data *dataMap) read() error {
	if (data.key == nil) {
		return errors.New("Can't load data withouth specifying which");
	}


	err := data.cacheGet();
	if nil == err {
		data.Print(" ==== MEMCACHE ==== READ: CACHE HIT FOR data " + data.entityName);
		return nil;
	}


	err = datastore.Get(data.context, data.key, data);

	data.Print(" ==== MEMCACHE ==== SET IN READ FOR data " + data.entityName);
	if nil == err {
		//didn't found the item. Put it into memcache anyhow
		data.cacheSet();
	}

	return err;
}

func (data *dataMap) update() error {
	if (data.key == nil) {
		return errors.New("Can't save a data that hasn't been loaded or created");
	}

	_, err := datastore.Put(data.context, data.key, data);

	if (nil != err) {
		return err;
	}

	data.Print(" ==== MEMCACHE ==== SET IN UPDATE FOR data " + data.entityName);

	data.cacheSet();

	//if item was
	data.Print("data " + data.entityName + " succesfully saved");

	return err;
}

func (model *Model) Update() error {

	e := model.dataMap.update();

	if nil == e && model.searchable{
		model.Index();
	}

	return e;
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

func (data *dataMap) AllOf() ([]Model, error) {

	prototype := data.Prototype();
	data.query = datastore.NewQuery(nameOfPrototype(prototype));

	limit := default_entry_count_per_read_batch;

	var models []Model;

	var cursor *datastore.Cursor;
	var e error;

	done := false;
	for !done {

		data.query = data.query.Limit(limit);

		if cursor != nil {
			data.query = data.query.Start(*cursor);
		}

		cursor, e = data.get(&models);

		done = e == datastore.Done;
	}

	data.query = nil;
	return models, nil;
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

func (data *dataMap) GetMulti() ([]Model, error) {
	//check if struct contains the fields
	const batched int = 1000;

	count, err := data.Count();

	if err != nil {
		return nil, err;
	}

	div := (count / batched);
	if (count % batched != 0) {
		div++;
	}

	log.Debugf(data.context, "found (count) %d items. div is %v", count, div);
	//create the blueprint
	prototype := data.Prototype();
	mtype := reflect.ValueOf(prototype).Elem().Type();
	val , _ := reflect.New(mtype).Interface().(Prototype);
	mm, _ := NewModel(data.context, val);
	mm.loadRef = false;

	//allocates memory for the resulting array
	res := make([]Model, count, count);

	var chans []chan []*dataMap;

	//retrieve items in concurrent batches
	mutex := new(sync.Mutex);
	for paging := 0; paging < div; paging++ {
		c := make(chan []*dataMap);

		go func(page int, ch chan []*dataMap, ctx context.Context) {

			log.Debugf(data.context, "Batch #%d started", page);
			offset := page * batched;

			rq := batched;
			if page + 1 == div {
				//we need the exact number o GAE will complain since len(dst) != len(keys) in getAll
				rq = count % batched;
			}

			//copy the data query into the local copy
			//dq := datastore.NewQuery(nameOfPrototype(data.Prototype()));
			dq := data.query;
			if data.query == nil {
				dq = datastore.NewQuery(nameOfPrototype(data.Prototype()));
			}
			dq = dq.Offset(offset);
			dq = dq.KeysOnly();

			keys := make([]*datastore.Key, rq, rq);
			partial := make(dataMaps, rq, rq);

			done := false;

			//Lock the loop or else other goroutine will starve the go scheduler causing a datastore timeout.
			mutex.Lock();
			c := 0;
			var cursor *datastore.Cursor;
			for !done {

				dq = dq.Limit(200);
				//right count
				if cursor != nil {
					//since we are using start, remove the offset, or it will count from the start of the query
					dq = dq.Offset(0);
					dq = dq.Start(*cursor);
				}

				it := dq.Run(data.context);

				for {

					key, err := it.Next(nil);

					if (err == datastore.Done) {
						break;
					}

					if err != nil {
						panic(err);
					}

					dm := &dataMap{};
					*dm = *mm.dataMap;
					dm.m = reflect.New(mtype).Interface().(Prototype);

					dm.key = key;

					log.Debugf(data.context, "c counter has value #%d. Max is %d, key is %s", c, rq, key.Encode());
					//populates the dst
					partial[c] = dm;
					//populate the key
					keys[c] = key;
					c++;
				}

				if c == rq {
					//if there are no more entries to be loaded, break the loop
					done = true;
					log.Debugf(data.context, "Batch #%d got %d keys from query", page, c);
				} else {
					//else, if we still have entries, update cursor position
					newCursor, e := it.Cursor();
					if e != nil {
						panic(err);
					}
					cursor = &newCursor;
				}
			}
			mutex.Unlock();

			fromCache, err := partial.cacheGetMulti(ctx, keys);

			if err != nil {
				log.Errorf(ctx, "Error retrieving multi from cache: %v", err);
			}

			c = 0;
			if len(fromCache) < rq {

				leftCount := len(keys) - len(fromCache);

				remainingKeys := make([]*datastore.Key, leftCount, leftCount);
				dst := make([]*dataMap, leftCount, leftCount);

				for i, k := range keys {
					_, ok := fromCache[k];
					if !ok {
						//add the pointer to those keys that have to be retrieved
						remainingKeys[c] = k;
						dst[c] = partial[i];
						c++;
					}
				}

				err = datastore.GetMulti(data.context, remainingKeys, dst);

				if err != nil {
					panic(err);
				}
			}

			log.Debugf(data.context, "Batch #%d retrieved all items. %d items retrieved from cache, %d items retrieved from datastore", page, len(fromCache), c);
			//now load the references of the model

			//todo: rework because it is not setting references.
//			partial.cacheSetMulti(ctx);

			ch <- partial;

		} (paging, c, data.context);

		chans = append(chans, c);
	}

	offset := 0;
	for _ , c := range chans {
		partial := <- c;
		for j , dm := range partial {
			m := Model{dataMap: dm, searchable:mm.searchable};
			res[offset + j] = m;
		}

		offset += len(partial);
		close(c);
	}

	return res, nil;
}


func (data *dataMap) Get() ([]Model, error) {

	if (data.query == nil) {
		return nil, errors.New("Can't get unspecified models.");
	}

	data.query = data.query.KeysOnly();

	var models []Model;

	_, e := data.get(&models);

	if e != nil && e != datastore.Done {
		return models, e;
	}

	//data.Print("DONE RECOVERING ITEMS. FOUND " + strconv.Itoa(rc) + " ITEMS FOR ENTITY " + data.entityName);
	//log.Printf("GET CALLED. Models: %+v", models);
	data.query = nil;
	return models, nil;
}

func (data *dataMap) get(models *[]Model) (*datastore.Cursor, error) {

	prototype := data.Prototype();
	mType := reflect.ValueOf(prototype).Elem().Type();

	more := false;
	rc := 0;
	it := data.query.Run(data.context);
	for {

		key, err := it.Next(nil);

		if (err == datastore.Done) {
			break;
		}

		if err != nil {
			return nil, err;
		}

		dst := reflect.New(mType);
		val, ok := dst.Interface().(Prototype);

		if !ok {
			data.query = nil;
			return nil, errors.New("Can't cast interface to Prototype");
		}

		more = true;

		model, err := NewModel(data.context, val);

		//log.Printf("RUNNING QUERY %v FOR MODEL " + data.entityName + " - FOUND ITEM WITH KEY: " + strconv.Itoa(int(key.IntID())), data.query);

		if (err != nil) {
			data.query = nil;
			return nil, err;
		}

		model.key = key;

		err = model.read();

		if (err != nil) {
			data.query = nil;
			return nil, err;
		}

		*models = append(*models, *model);
		rc++;
	}

	if !more {
		//if there are no more entries to be loaded, break the loop
		return nil, datastore.Done;
	} else {
		//else, if we still have entries, update cursor position
		cursor, e := it.Cursor();

		return &cursor, e;
	}
}

//convenience method to retrieve first occurrence of model after a get
func (model *Model) First() error {

	mods, err := model.Get();

	if err != nil {
		return err;
	}

	for _, v := range mods {
		*model = v;
		return nil;
	}

	return datastore.ErrNoSuchEntity;
}

func (model *Model) With(filter string, value interface{}) {
	dq := model.query;
	//initialize query if not initialized
	if nil == dq {
		dq = datastore.NewQuery(nameOfPrototype(model.Prototype()));
	}

	dq = dq.Filter(filter, value);
	model.query = dq;
}

func (model *Model) OrderBy(fieldName string) {
	dq := model.query;
	if nil == dq {
		dq = datastore.NewQuery(nameOfPrototype(model.Prototype()));
	}

	dq = dq.Order(fieldName);
	model.query = dq;
}

func (model *Model) OffsetBy(offset int) {
	dq := model.query;
	if nil == dq {
		dq = datastore.NewQuery(nameOfPrototype(model.Prototype()));
	}

	dq = dq.Offset(offset);
	model.query = dq;
}

func (model *Model) Limit(limit int) {
	dq := model.query;
	if nil == dq {
		dq = datastore.NewQuery(nameOfPrototype(model.Prototype()));
	}

	dq = dq.Limit(limit);
	model.query = dq;
}

func (data *dataMap) Count() (int, error) {
	dq := data.query;
	if nil == dq {
		dq = datastore.NewQuery(nameOfPrototype(data.Prototype()));
	}
	return dq.Count(data.context);
}

func (model *Model) WithReference(ref Model) {
	if (ref.key == nil) {
		//try to load the reference query
		panic("Can't search by unexisting reference - key not set");
	}

	err := model.SetReference(ref);
	if nil != err {
		panic(err);
	}
	refName := makeRefname(ref.entityName);
	model.Print("==== WITH REFERENCE ==== with key " + refName);

	model.With(refName + " = ", ref.key);
}

func (model *Model) ByID(id int64) error {
	//first try to retrieve item from memcache

	model.key = datastore.NewKey(model.context, model.entityName, "", id, nil);
	return model.read();
}

func (model Model) Name() string {
	return model.entityName;
}

//returns -1 if the model doesn't have an id
//returns the id of the model otherwise
func (model Model) Id() int64 {
	if model.key == nil {
		return -1;
	}

	return model.key.IntID();
}

func (data dataMap) Prototype() Prototype {
	return data.m;
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
