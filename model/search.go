package model

/*import (
	"google.golang.org/appengine/search"
	"reflect"
	"strings"

	"errors"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
	//"log"
)

//flag fields that we want to search with "prototype:search"
const tag_search string = "search";

func (model *Model) Save() ([]search.Field, *search.DocumentMetadata, error) {

	var fields []search.Field;

	model.parseSearchTags(&fields, nil);

	if len(fields) > 0 {
		return fields, nil, nil;
	}

	return nil, nil, nil;
}

func (model *Model) parseSearchTags(searchFields *[]search.Field, meta *search.DocumentMetadata) {
	proto := reflect.ValueOf(model.m).Elem();
	for i := 0; i < proto.NumField(); i++ {
		field := proto.Type().Field(i);

		name, _ := field.Tag.Get(tag_domain), ""
		if i := strings.Index(name, ","); i != -1 {
			name, _ = name[:i], name[i+1:]
		}

		//if the field is eligible for searching:
		if name == tag_search {

			ptr := proto.Field(i).Interface();
			zero := reflect.Zero(field.Type).Interface();

			if ptr == zero {
				continue;
			}

			sf := search.Field{};
			sf.Name = field.Name;

			switch field.Type.Kind() {
				case reflect.Bool:
					sf.Value = proto.Field(i).Bool();
				case reflect.String:
					sf.Value = proto.Field(i).String();
				case reflect.Struct:
				//if we have a reference, than save the id of the ref
					ref, ok := model.references[i];

					if !ok {
						continue;
					}
					//should always have a ref with key here, since it's called after create/update
					sf.Value = search.Atom(ref.key.Encode());
				default:
					continue;
			}

			*searchFields = append(*searchFields, sf);
		}
	}
}

func (model Model) Index() error {
	if !model.searchable {
		return errors.New("Model is not searchable");
	}

	index, err := search.Open(model.entityName);
	if nil != err {
		panic(err);
	}

	_, err = index.Put(model.context, model.key.Encode(), &model);

	return err;
}

func (model *Model) deleteSearch() error {
	index, err := search.Open(model.entityName);
	if nil != err {
		panic(err);
	}

	return index.Delete(model.context, model.key.Encode());
}

func (model *Model) Clean() (int, error) {
	index, err := search.Open(model.entityName);
	if nil != err {
		return 0, err;
	}
	opts := &search.ListOptions{};
	opts.IDsOnly = true;

	cleaner, _ := NewModel(model.context, model.Prototype());
	count := 0;
	for it := index.List(model.context, opts); ; {
		k, e := it.Next(nil);

		if e == search.Done {
			break;
		}

		key, err := datastore.DecodeKey(k);

		if err != nil {
			return count, errors.New("Can't deliver search result. Keys do not match! - " + k);
		}

		cleaner.key = key;

		e = datastore.Get(cleaner.context, cleaner.key, cleaner.dataMap);

		if e != nil {
			//if we can't read the item, we delete the index
			count ++;
			index.Delete(model.context, k);
			log.Infof(model.context, "Removed stale index");
		}
	}

	return count, nil;
}


//stays at nil -> ignores the struct datas and gets a key only query from datastore
//which will load the struct with Read()
func (model *Model) Load(fields []search.Field, meta *search.DocumentMetadata) error {
	return nil;
}

func (model *Model) SearchWith(query string) {
	model.searchQuery += query;
}

//so far, op is the logical operation to use with the reference, i.e. AND, OR.
//with reference is always an equality
func (model *Model) SearchWithRef(ref *Model, op string) error {

	refName := makeRefname(ref.entityName);
	s, ok := model.values[refName];
	if !ok {
		model.Dump();
		return errors.New("No reference named " + ref.entityName + " for model " + model.Name());
	}
	pval := reflect.ValueOf(model.Prototype()).Elem();
	fieldName := pval.Type().Field(s.index).Name;

	if op != "" {
		op = strings.TrimSpace(op);
		model.searchQuery += " " + op + " ";
	}

	model.searchQuery += " " + fieldName + " = " + ref.key.Encode();

	return nil;
}

func (model *Model) Search(opts *search.SearchOptions) (int, []Model, error) {

	//always do a id-only key. retrieval is demanded to model
	if nil == opts {
		opts = &search.SearchOptions{};
	}

	opts.IDsOnly = true;
	//see if we want to search in a reference. Empty string means we don't want to
	mod := model;
	var err error;

	index, err := search.Open(mod.entityName);

	if err != nil {
		panic(err);
	}

	var mods []Model;

	mType := reflect.ValueOf(model.m).Elem().Type();

	query := model.searchQuery;
	//log.Print("****** SEARCH QUERY: " + query);
	log.Debugf(model.context, "****** SEARCH QUERY: " + query);
	model.searchQuery = "";

	count := 0;
	for it := index.Search(model.context, query, opts); ; {


		k, e := it.Next(nil);

		count = it.Count();
		if e == search.Done {
			log.Debugf(model.context, "****** SEARCH FOUND: %d ITEMS", count);
			break;
		}

		dst := reflect.New(mType);
		val, ok := dst.Interface().(Prototype);

		if !ok {
			return count, nil, errors.New("Can't cast search result to prototype");
		}

		m, err := NewModel(model.context, val);

		if nil != err {
			return count, nil, err;
		}

		key, err := datastore.DecodeKey(k);

		if err != nil {
			return count, nil, errors.New("Can't deliver search result. Keys do not match! - " + k);
		}

		m.key = key;

		e = m.read();

		if e != nil {
			return count, nil, e;
		}

		mods = append(mods, *m);
	}

	if len(mods) < 1 {
		err = search.ErrNoSuchDocument;
	}

	return count, mods, err;

}*/