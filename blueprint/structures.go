package blueprint


import (
	"reflect"
	"log"
	"time"
	"fmt"
	"strings"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine"
	"errors"
)


var (
	typeOfModel = reflect.TypeOf(Model{})
	typeOfStructure = reflect.TypeOf(structure{})
)



//struct value represent a struct that internally can map other structs
//fieldIndex is the index of the struct
//if isReference is true, the struct must be considered as a separate model
type encodedField struct {
	index int
	childStruct *encodedStruct
	tag string
}

type encodedStruct struct {
	structName string
	fieldNames map[string]encodedField
}

func newEncodedStruct() *encodedStruct {
	mp := make(map[string]encodedField)
	return &encodedStruct{"",mp}
}

func mapStructure(t reflect.Type, s *encodedStruct, parentName string) {
	log.Printf("====MAP STRUCT==== Analyzing struct of type %s and kind %s with parent %s",t.Name(), t.Kind(), parentName);
	if t == typeOfModel || t == typeOfStructure {
		log.Printf("====MAP STRUCT==== skipping struct of type %s", t.Name());
		return
	}

	//iterate over struct props
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i);

		//skip unexported fields
		if field.PkgPath != "" {
			log.Printf("====MAP STRUCT==== skipping unexported field %s", field.Name);
			continue
		}

		//skip model mapping in field
		if field.Type == typeOfModel {
			continue
		}

		tags := strings.Split(field.Tag.Get(tag_domain), ",")
		tagName := tags[0]
		//todo: skip also datastore skippable data
		if tagName == tag_skip {
			continue
		}

		sName := parentName + "." + field.Name
		sValue := encodedField{index:i}

		log.Printf("====MAP STRUCT==== Processing field %s of struct %s", field.Name, t.Name())

		switch field.Type.Kind() {
			case reflect.Map:
			fallthrough
			case reflect.Array:
			continue
			case reflect.Slice:
				//todo: se è uno slice di struct, trattali tutti alla stessa maniera,
				//notifica a GAE che è uno slice usando property.multiple in save/load
				//pensare a come rappresentare nella mappa uno slice.
				//todo::if here, nested slice so not supported
			case reflect.Struct:

				//we already mapped the struct, skip further computations
				if _, ok := encodedStructs[field.Type]; ok {
					log.Printf("!!!Struct of type %s already mapped. Using mapped value %v", field.Type, encodedStructs[field.Type])
					sValue.childStruct = encodedStructs[field.Type]
					sValue.childStruct.structName = sName
					continue
				}

				//else we map the other struct
				sMap := make(map[string]encodedField);
				childStruct := &encodedStruct{structName:sName, fieldNames:sMap};
				sValue.childStruct = childStruct;
				mapStructure(field.Type, childStruct, field.Type.Name());
			break;
			case reflect.Ptr:
				//if we have a pointer we store the value it points to
				fieldElem := field.Type.Elem()

				if fieldElem.Kind() == reflect.Struct {
					sMap := make(map[string]encodedField);
					childStruct := &encodedStruct{structName:sName, fieldNames:sMap};
					sValue.childStruct = childStruct;
					mapStructure(fieldElem, childStruct, field.Type.Name());
					break
				}
			fallthrough
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

		s.fieldNames[sName] = sValue;
		log.Printf("Struct of type %s has fieldName %s\n", t.Name(), sName)
	}
	encodedStructs[t] = s;
}


func encodeStruct(s interface{}, props *[]datastore.Property, multiple bool, codec *encodedStruct) error {

	name := codec.structName;
	value := reflect.ValueOf(s).Elem();
	sType := value.Type();

	for i := 0; i < sType.NumField(); i++ {
		field := sType.Field(i);


		if field.Tag.Get("datastore") == "-" {
			continue;
		}

		v := value.FieldByName(field.Name);

		p := &datastore.Property{};

		p.Multiple = multiple;


		p.Name = name + "." + field.Name;


		gPrint("==== SAVE ==== encoding field " + p.Name);
		gPrintf("==== SAVE ==== interface of type %+v", v.Interface());
		switch x := v.Interface().(type) {
			case time.Time:
				p.Value = x
			case appengine.BlobKey:
				p.Value = x
			case appengine.GeoPoint:
				p.Value = x
			case datastore.ByteString:
				p.Value = x
			default:
				switch v.Kind() {
					case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
						p.Value = v.Int();
					case reflect.Bool:
						p.Value = v.Bool()
					case reflect.String:
						p.Value = v.String()
					case reflect.Float32, reflect.Float64:
						p.Value = v.Float()
					case reflect.Slice:
						p.Multiple = true;

						if v.Type().Elem().Kind() != reflect.Uint8 {
							if val, ok := codec.fieldNames[field.Name]; ok {
								for j := 0; j < v.Len(); j++ {
									gPrint("==== SAVE ==== slice value for " + field.Name + " is of type " + v.Index(j).Type().Name());
									gPrintf("=== SAVE === slice elem passing is %+v ", v.Index(j).Addr().Interface() );
									if err := encodeStruct(v.Index(j).Addr().Interface(), props, true, val.childStruct); err != nil {
										panic(err);
									}
								}
								break;
							}
						}

						p.NoIndex = true
						p.Value = v.Bytes()
					case reflect.Struct:
						gPrint("==== SAVE ==== encoding struct " + p.Name);
						if !v.CanAddr() {
							return fmt.Errorf("datastore: unsupported struct field %s for entity of type %s: value %v is unaddressable",p.Name, sType, v)
						}

						if field.Type == typeOfTime || field.Type == typeOfGeoPoint {

						}

						for k, _ := range codec.fieldNames {
							gPrint("==== SAVE ==== coded " + codec.structName + " has key: " + k + " - property name is: " + p.Name);
						}
						if val, ok := codec.fieldNames[p.Name]; ok {
							if nil != val.childStruct {
								encodeStruct(v.Addr().Interface(), props, p.Multiple, val.childStruct);
								continue;
							}
							return fmt.Errorf("Inconsistent model. Struct child inequivalence");
						}
						//if struct, recursively call itself until an error is found

						return fmt.Errorf("FieldName % s not found in %v for Entity of type %s", p.Name, codec.fieldNames, sType);
				}
		}

		*props = append(*props, *p);
		gPrintf("==== SAVE ==== saved props %+v ", *props);

	}

	return nil;
}

type propertyLoader struct {
	mem map[string]int
}

//parentEncodedField represents a field of interface{} s
func (l *propertyLoader) decodeField(s reflect.Value, p datastore.Property, encodedField encodedField) error {

	interf := s;
	if (s.Kind() == reflect.Ptr) {
		interf = s.Elem();
	}

	//todo::handle slice exception case where slice of slices

	//get the field we are decoding
	field := interf.Field(encodedField.index);
	fname := interf.Type().Field(encodedField.index).Name;

	//gPrintf("%+v\n", interf);
	gPrint("==== DECODE STRUCT ==== processing Property " + p.Name + " for field " + fname + " OF STRUCT WITH TYPE " + field.Kind().String());

	switch field.Kind() {
	//if the field is a struct it can either be a special value (time or geopoint) OR a struct that we have to decode
		case reflect.Struct:
			//todo: in encoding the model, treat time and geopoint as direct values
			switch field.Type() {
			case typeOfTime:
				x, ok := p.Value.(time.Time)
				if !ok && p.Value != nil {
					return errors.New("Error - Invalid Time type");
				}
				field.Set(reflect.ValueOf(x))
			case typeOfGeoPoint:
				x, ok := p.Value.(appengine.GeoPoint)
				if !ok && p.Value != nil {
					return errors.New("Error - invalid geoPoint type");
				}
				field.Set(reflect.ValueOf(x))
			default:
				//case of struct to decode
				//instantiate a new struct of the type of the field v

				for k, _ := range encodedField.childStruct.fieldNames {
					gPrint("==== DECODE STRUCT ==== Decoded struct has field: " + k);
				}

				//populate the struct - MUST POPULATE THE FIELD ITSELF!

				//get the encoded field for the attr of the struct with name == p.Name
				if attr, ok := encodedField.childStruct.fieldNames[p.Name]; ok {
					gPrint("==== DECODE STRUCT ==== FOUND NESTED STRUCT " + encodedField.childStruct.structName +
					" for Property "+ p.Name + " AND FOR FIELD " + field.Type().Field(attr.index).Name);

					l.decodeField(field.Addr(), p, attr)
				}

				//else go down one level
				baseName := baseName(p.Name);
				gPrint("SUB-NAME ----->>>>>" +  baseName);
				if attr, ok := encodedField.childStruct.fieldNames[baseName]; ok {
					gPrint("==== DECODE STRUCT ==== FOUND NESTED STRUCT " + encodedField.childStruct.structName +
					" for Property "+ p.Name + " AND FOR FIELD " + field.Type().Field(attr.index).Name);

					l.decodeField(field.Addr(), p, attr);
				}

				return nil;
			}

		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			x, ok := p.Value.(int64)
			if !ok && p.Value != nil {
				return errors.New("Error 1");
			}
			if field.OverflowInt(x) {
				return fmt.Errorf("value %v overflows struct field of type %v", x, field.Type());
			}
			field.SetInt(x)
		case reflect.Bool:
			x, ok := p.Value.(bool)
			if !ok && p.Value != nil {
				return errors.New("Error 2");
			}
			field.SetBool(x)
		case reflect.String:
			switch x := p.Value.(type) {
			case appengine.BlobKey:
				field.SetString(string(x));
			case datastore.ByteString:
				field.SetString(string(x));
			case string:
				field.SetString(x);
				gPrint("==== DECODE STRUCT ==== Set string with value: " + x + " for field " + fname);
			default:
				if p.Value != nil {
					return errors.New("Error 3");
				}
			}
		case reflect.Float32, reflect.Float64:
			x, ok := p.Value.(float64)
			if !ok && p.Value != nil {
				return errors.New("Error 4");
			}
			if field.OverflowFloat(x) {
				return fmt.Errorf("value %v overflows struct field of type %v", x, field.Type());
			}
			field.SetFloat(x)
		case reflect.Ptr:
			//throw an exception here since keys are not supported directly by the model framework
			return fmt.Errorf("Pointer type not supported. Found Pointer for field %v", field.Type());
		case reflect.Slice:
			x, ok := p.Value.([]byte)
			if !ok {
				if y, yok := p.Value.(datastore.ByteString); yok {
					x, ok = []byte(y), true
				}
			}
			if !ok && p.Value != nil {
				//if it's a struct slice
				if !p.Multiple {

					return errors.New("Error - invalid slice. Can only support byte slices (Bytestrings)");
				}
			}

			if field.Type().Elem().Kind() != reflect.Uint8 {

				if l.mem == nil {
					l.mem = make(map[string]int)
				}
				index := l.mem[p.Name];
				l.mem[p.Name] = index + 1;
				for field.Len() <= index {
					sliceElem := reflect.New(field.Type().Elem()).Elem();
					gPrintf("==== DECODE STRUCT SLICE ==== Type of slice is %q", sliceElem.Type().String());
					field.Set(reflect.Append(field, sliceElem));
				}
				l.decodeField(field.Index(index), p, encodedField.childStruct.fieldNames[p.Name]);
				gPrintf("loader: %+v", l.mem);
				//return errors.New("Error - NOT SUPPORTING SIMPLE SLICES AS OF YET");
				break;
			}

			field.SetBytes(x)
		default:
			return fmt.Errorf("unsupported load type %s", field.Kind().String());
	}

	return nil;
}

//takes a property field name and returns it's base
func baseName(name string) string {
	//get the last index of the separator
	lastIndex := strings.LastIndex(name, val_serparator);
	if lastIndex > 0 {
		return name[0:lastIndex];
	}
	return name;
}

func pureName(fullName string) string {
	lastIndex := strings.LastIndex(fullName, val_serparator);
	if lastIndex > 0 {
		return fullName[lastIndex + 1:];
	}
	return fullName;
}


func toPropertyList(modelable modelable) ([]datastore.Property, error) {

	value := reflect.ValueOf(modelable).Elem();
	sType := value.Type();

	model := modelable.getModel();

	var props []datastore.Property;

	//loop through prototype fields
	//and handle them accordingly to their type
	for i := 0; i < sType.NumField(); i++ {
		field := sType.Field(i);

		if field.Type == typeOfModel {
			continue
		}

		if field.Tag.Get("datastore") == "-" {
			continue;
		}

		p := &datastore.Property{};

		p.Name = sType.Name() + "." + field.Name;

		if rm, ok := model.references[i]; ok {
			ref := rm.getModel();

			//pass reference types to datastore as *datastore.Key type
		//	name := ref_model_prefix + ref.structName;
			//p := datastore.Property{Name:name, Value:ref.key};
			p.Value = ref.key;
			props = append(props, *p);
			continue
		}

		v := value.Field(i);

		switch x := v.Interface().(type) {
		case time.Time:
			p.Value = x
		case appengine.BlobKey:
			p.Value = x
		case appengine.GeoPoint:
			p.Value = x
		case datastore.ByteString:
			p.Value = x
		default:
			switch v.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				p.Value = v.Int();
			case reflect.Bool:
				p.Value = v.Bool()
			case reflect.String:
				p.Value = v.String()
			case reflect.Float32, reflect.Float64:
				p.Value = v.Float()
			case reflect.Slice:
				p.Multiple = true;

				if v.Type().Elem().Kind() != reflect.Uint8 {
					if val, ok := model.fieldNames[p.Name]; ok {
						for j := 0; j < v.Len(); j++ {
							if err := encodeStruct(v.Index(j).Addr().Interface(), &props, true, val.childStruct); err != nil {
								panic(err);
							}
						}
						continue;
					}
				}

				p.NoIndex = true
				p.Value = v.Bytes()

			case reflect.Struct:
				if !v.CanAddr() {
					return nil, fmt.Errorf("datastore: unsupported struct field %s: value is unaddressable", field.Name)
				}
				//if struct, recursively call itself until an error is found
				//as debug, check consistency. we should have a value at i
				if val, ok := model.fieldNames[p.Name]; ok {
					err := encodeStruct(v.Addr().Interface(), &props, false, val.childStruct);
					if err != nil {
						panic(err);
					}
					continue;
				}

				return nil, fmt.Errorf("FieldName % s not found in %v for Entity of type %s", p.Name, model.fieldNames, sType);
			}
		}

		props = append(props, *p);

	}
	return props, nil;
}


func fromPropertyList(modelable modelable, props []datastore.Property) error {

	//get the underlying prototype
	value := reflect.ValueOf(modelable).Elem();
	sType := value.Type();
	model := modelable.getModel();

	for _, p := range props {
		//field is the value of a struct field
		//var field reflect.Value;
		//check if prop is in data base struct
		//log.gPrint("==== LOAD MODEL ==== Field of prop " + p.Name + " HAS KIND: " + field.Kind().String());
		log.Printf("==== LOAD MODEL ==== Reading prop with name %s", p.Name)

		//if we have a reference
		if key, ok := p.Value.(*datastore.Key); ok {
			log.Printf("Found datastore.key for field %s", p.Name);
			/*for i := 0; i < sType.NumField(); i++ {
				f := sType.Field(i);
				log.Printf("Struct of type %s has field with name %s.", sType, f.Name)
			}*/

			if field, ok := sType.FieldByName(pureName(p.Name)); ok {
				//Note: understand what is the deal with []int in index field.
				ref := model.references[field.Index[0]];
				rm := ref.getModel();
				rm.key = key;
				continue
			}
			return fmt.Errorf("No reference found at name %s for type %s. PureName is %s", p.Name, sType, pureName(p.Name));
		}

		//load first level values.
		if attr, ok := model.fieldNames[p.Name]; ok {

			/*if (attr.isReference) {
				ref, ok := data.references[attr.index];
				if !ok {
					panic("Error - Unconsistent data - should have a reference at index: " + strconv.Itoa(attr.index));
				}
				//we found a reference, load the data and assign the struct to the field
				key, ok := p.Value.(*datastore.Key);

				if !ok && p.Value != nil {
					panic("Error - Unconsistent data - should have a reference at index " + strconv.Itoa(attr.index) + " for field with name " + p.Name);
				}

				field := value.Field(attr.index);
				//it's alrgight, we have a key for a reference. Assign the key to the corrisponding data and load it
				ref.key = key;
				if data.loadRef {
					err := ref.read();

					if nil != err {
						return err;
					}
				}

				field.Set(reflect.ValueOf(ref.m).Elem());
				continue
			}*/

			//decode the field if its a plain value (no struct, no pointer, no slice, not sure about map)
			if attr.childStruct == nil {
				err := model.decodeField(reflect.ValueOf(modelable), p, attr);
				if nil != err {
					return err;
				}
				continue;
			}
		}

		//if is not in the first level get the first level name
		//firstLevelName := strings.Split(p.Name, ".")[0];
		if attr, ok := model.fieldNames[pureName(p.Name)]; ok {
			//v := reflect.ValueOf(modelable).Elem();
			//field := v.Field(attr.index);
			//data.Print("==== LOAD MODEL ==== NESTED FIELD OF KIND " + field.Kind().String() + " FOR PROPERTY "  + p.Name );
			//gPrint("==== LOAD MODEL ==== PROPERTY " + p.Name + " IS OF TYPE NESTED VALUE - FIRST LEVEL NAME IS: " + firstLevelName);
			err := model.decodeField(reflect.ValueOf(modelable), p, attr);

			if nil != err {
				return err;
			}
		} else {
			log.Printf("Field with name %s not found for struct of type %s", pureName(p.Name), sType);
		}
	}

	return nil;
}