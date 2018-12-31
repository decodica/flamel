package model

import (
	"errors"
	"fmt"
	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"reflect"
	"strings"
	"sync"
	"time"
)

//Define special reflect.Type
var (
	typeOfGeoPoint = reflect.TypeOf(appengine.GeoPoint{})
	typeOfTime = reflect.TypeOf(time.Time{})
	typeOfModel = reflect.TypeOf(Model{})
	typeOfModelable = reflect.TypeOf((*modelable)(nil)).Elem()
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

//Keeps track of encoded structs according to their reflect.Type.
//It is used as a cache to avoid to map structs that have been already mapped
var encodedStructsMutex sync.Mutex
var encodedStructs = map[reflect.Type]*encodedStruct{}

func mapStructure(t reflect.Type, s *encodedStruct, parentName string) {
	encodedStructsMutex.Lock()
	mapStructureLocked(t, s, parentName)
	encodedStructsMutex.Unlock()
}
//maps a structure into a linked list representation of its fields.
//It is used to ease the conversion between the Model framework and the datastore
func mapStructureLocked(t reflect.Type, s *encodedStruct, parentName string) {
	if t == typeOfModel || t == typeOfStructure {
		return
	}

	//iterate over struct props
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fType := field.Type

		//skip unexported fields
		if field.PkgPath != "" {
			continue
		}

		//skip model mapping in field
		if fType == typeOfModel {
			continue
		}

		tags := strings.Split(field.Tag.Get(tagDomain), ",")
		tagName := tags[0]

		if tagName == tagSkip {
			continue
		}

		sName := referenceName(parentName, field.Name)
		sValue := encodedField{index:i}
		switch fType.Kind() {
			case reflect.Map:
			fallthrough
			case reflect.Array:
			continue
			case reflect.Slice:
				//todo: validate supported slices
				//notifica a GAE che è uno slice usando property.multiple in save/load
				//pensare a come rappresentare nella mappa uno slice.
				//todo::if here, nested slice so not supported
				fType = field.Type.Elem()
				if fType.Kind() != reflect.Struct {
					break
				}
			fallthrough;
			case reflect.Struct:
				//we already mapped the struct, skip further computations
				//else we map the other struct
				if _, ok := encodedStructs[fType]; ok {
					sValue.childStruct = encodedStructs[fType]
					sValue.childStruct.structName = sName
					break
				}
				//todo: add to map anyway, so that the second time we get a cached value
				sMap := make(map[string]encodedField)
				childStruct := &encodedStruct{structName:sName, fieldNames:sMap}
				sValue.childStruct = childStruct;
				if reflect.PtrTo(fType).Implements(typeOfModelable) {
					//we have a reference, so we must treat it as a root struct
					mapStructureLocked(fType, childStruct, "")
				} else {
					mapStructureLocked(fType, childStruct, field.Name)
				}
			break;
			case reflect.Ptr:
				//if we have a pointer we store the value it points to
				fieldElem := fType.Elem()
				if fieldElem.Kind() == reflect.Struct {
					sMap := make(map[string]encodedField)
					childStruct := &encodedStruct{structName:sName, fieldNames:sMap}
					sValue.childStruct = childStruct
					mapStructureLocked(fieldElem, childStruct, field.Name)
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

		s.fieldNames[sName] = sValue
	}
	encodedStructs[t] = s
}

func encodeStruct(s interface{}, props *[]datastore.Property, multiple bool, codec *encodedStruct) error {
	name := codec.structName
	value := reflect.ValueOf(s).Elem()
	sType := value.Type()

	for i := 0; i < sType.NumField(); i++ {
		field := sType.Field(i)

		if field.Tag.Get("datastore") == "-" {
			continue;
		}

		v := value.FieldByName(field.Name)
		p := &datastore.Property{}
		p.Multiple = multiple

		if p.Multiple {
			p.NoIndex = true
		}

		p.Name = referenceName(name, field.Name)
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
						p.Value = v.Int()
					case reflect.Bool:
						p.Value = v.Bool()
					case reflect.String:
						p.Value = v.String()
					case reflect.Float32, reflect.Float64:
						p.Value = v.Float()
					case reflect.Slice:
						p.Multiple = true
						if v.Type().Elem().Kind() != reflect.Uint8 {
							if val, ok := codec.fieldNames[field.Name]; ok {
								for j := 0; j < v.Len(); j++ {
									if err := encodeStruct(v.Index(j).Addr().Interface(), props, true, val.childStruct); err != nil {
										panic(err)
									}
								}
								break
							}
						}
						p.NoIndex = true
						p.Value = v.Bytes()
					case reflect.Struct:
						if !v.CanAddr() {
							return fmt.Errorf("datastore: unsupported struct field %s for entity of type %s: value %v is unaddressable",p.Name, sType, v)
						}

						if val, ok := codec.fieldNames[p.Name]; ok {
							if nil != val.childStruct {
								encodeStruct(v.Addr().Interface(), props, p.Multiple, val.childStruct)
								continue
							}
							return fmt.Errorf("struct %s is not a field of codec %+v", p.Name, codec)
						}
						//if struct, recursively call itself until an error is found
						return fmt.Errorf("FieldName %s not found in %v for Entity of type %s", p.Name, codec.fieldNames, sType)
				}
		}
		*props = append(*props, *p)
	}
	return nil
}

type propertyLoader struct {
	mem map[string]int
}

//parentEncodedField represents a field of interface{} s
func decodeStruct(s reflect.Value, p datastore.Property, encodedField encodedField, l *propertyLoader) error {
	interf := s
	if s.Kind() == reflect.Ptr {
		interf = s.Elem()
	}
	//todo::handle slice exception case where slice of slices

	//get the field we are decoding
	field := interf.Field(encodedField.index)
	switch field.Kind() {
	//if the field is a struct it can either be a special value (time or geopoint) OR a struct that we have to decode
	case reflect.Struct:
		//todo: in encoding the model, treat time and geopoint as direct values
		switch field.Type() {
		//ignore the model struct itself
		case typeOfModel:
		case typeOfTime:
			x, ok := p.Value.(time.Time)
			if !ok && p.Value != nil {
				return errors.New("error - Invalid Time type")
			}
			field.Set(reflect.ValueOf(x))
		case typeOfGeoPoint:
			x, ok := p.Value.(appengine.GeoPoint)
			if !ok && p.Value != nil {
				return errors.New("error - invalid geoPoint type")
			}
			field.Set(reflect.ValueOf(x))
		default:

			//instantiate a new struct of the type of the field v
			//get the encoded field for the attr of the struct with name == p.Name
			if attr, ok := encodedField.childStruct.fieldNames[p.Name]; ok {
				decodeStruct(field.Addr(), p, attr, l)
			}
			//else go down one level
			cName := childName(p.Name);
			if attr, ok := encodedField.childStruct.fieldNames[cName]; ok {
				decodeStruct(field.Addr(), p, attr, l)
			}
			return nil
		}
	case reflect.Slice:
		sliceKind := field.Type().Elem().Kind()

		x, ok := p.Value.([]byte)
		if !ok {
			if y, yok := p.Value.(datastore.ByteString); yok {
				x, ok = []byte(y), true
			}
		}
		if !ok && p.Value != nil {
			//if it's a struct slice
			if !p.Multiple {
				return errors.New("error - invalid slice. Can only support byte slices (Bytestrings)")
			}
		}

		if sliceKind != reflect.Uint8 {
			if l.mem == nil {
				l.mem = make(map[string]int)
			}
			index := l.mem[p.Name]
			l.mem[p.Name] = index + 1
			for field.Len() <= index {
				sliceElem := reflect.New(field.Type().Elem()).Elem()
				field.Set(reflect.Append(field, sliceElem))
			}

			if sliceKind == reflect.Struct {
				if attr, ok := encodedField.childStruct.fieldNames[p.Name]; ok {
					decodeStruct(field.Index(index), p, attr, l)
				}
				//else go down one level
				cName := childName(p.Name);
				if attr, ok := encodedField.childStruct.fieldNames[cName]; ok {
					decodeStruct(field.Index(index), p, attr, l)
				}
			} else {
				err := decodeField(field.Index(index), p)
				if err != nil {
					return err
				}
			}

			break
		}

		field.SetBytes(x)
	default:

		decodeField(field, p)
	}

	return nil
}

//todo define errors
func decodeField(field reflect.Value, p datastore.Property) error {

	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		x, ok := p.Value.(int64)
		if !ok && p.Value != nil {
			return errors.New("error 1")
		}
		if field.OverflowInt(x) {
			return fmt.Errorf("value %v overflows struct field of type %v", x, field.Type())
		}
		field.SetInt(x)
	case reflect.Bool:
		x, ok := p.Value.(bool)
		if !ok && p.Value != nil {
			return errors.New("error 2")
		}
		field.SetBool(x)
	case reflect.String:
		switch x := p.Value.(type) {
		case appengine.BlobKey:
			field.SetString(string(x))
		case datastore.ByteString:
			field.SetString(string(x))
		case string:
			field.SetString(x)
		default:
			if p.Value != nil {
				return errors.New("error 3")
			}
		}
	case reflect.Float32, reflect.Float64:
		x, ok := p.Value.(float64)
		if !ok && p.Value != nil {
			return errors.New("error 4")
		}
		if field.OverflowFloat(x) {
			return fmt.Errorf("value %v overflows struct field of type %v", x, field.Type())
		}
		field.SetFloat(x)
	case reflect.Ptr:
		x, ok := p.Value.(*datastore.Key)
		if !ok && p.Value != nil {
			return fmt.Errorf("unsupported load type %s", field.Type().String())
		}
		if _, ok := field.Interface().(*datastore.Key); !ok {
			return fmt.Errorf("unsupported pointer interface %s", field.Interface())
		}
		field.Set(reflect.ValueOf(x))
	default:
		return fmt.Errorf("unsupported load type %s", field.Kind().String())
	}
	return nil
}

//returns the name of a reference
func referenceName(parentName string, refName string) string {
	if parentName == "" {
		return refName
	}
	return parentName + "." + refName
}


func entityPropName(entityName string, fieldName string) string {
	return fieldName
	//return fmt.Sprintf("%s.%s", entityName, fieldName);
}

//takes a property field name and returns it's base
func baseName(name string) string {
	//get the last index of the separator
	lastIndex := strings.LastIndex(name, valSeparator)
	if lastIndex > 0 {
		return name[0:lastIndex]
	}
	return name
}

func pureName(fullName string) string {
	lastIndex := strings.LastIndex(fullName, valSeparator)
	if lastIndex > 0 {
		return fullName[lastIndex + 1:]
	}
	return fullName
}

func childName(fullName string) string {
	firstIndex := strings.Index(fullName, valSeparator)
	if firstIndex > 0 {
		return fullName[firstIndex + 1:]
	}
	return fullName
}

func toPropertyList(modelable modelable) ([]datastore.Property, error) {
	value := reflect.ValueOf(modelable).Elem()
	sType := value.Type()

	model := modelable.getModel()

	var props []datastore.Property
	//loop through prototype fields
	//and handle them accordingly to their type
	for i := 0; i < sType.NumField(); i++ {
		field := sType.Field(i)

		if field.Type == typeOfModel {
			continue
		}

		if field.Tag.Get("datastore") == "-" {
			continue
		}

		if field.Tag.Get("model") == tagSkip {
			continue
		}

		p := datastore.Property{}


		if field.Tag.Get("model") == tagNoindex {
			p.NoIndex = true
		}

		p.Name = referenceName("", field.Name)

		if rm, ok := model.references[i]; ok {
			ref := rm.Modelable.getModel()

			//pass reference types to datastore as *datastore.Key type
			//	name := ref_model_prefix + ref.structName;
			//p := datastore.Property{Name:name, Value:ref.key};
			p.Value = ref.Key
			props = append(props, p)
			continue
		}
		v := value.Field(i)
		switch x := v.Interface().(type) {
		case time.Time:
			p.Value = x
		case appengine.BlobKey:
			p.Value = x
		case appengine.GeoPoint:
			p.Value = x
		case datastore.ByteString:
			p.Value = x
		case *datastore.Key:
			p.Value = x
		default:
			switch v.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				p.Value = v.Int()
			case reflect.Bool:
				p.Value = v.Bool()
			case reflect.String:
				p.Value = v.String()
			case reflect.Float32, reflect.Float64:
				p.Value = v.Float()
			case reflect.Slice:
				sliceKind := v.Type().Elem().Kind()
				if sliceKind != reflect.Uint8 {

					if val, ok := model.fieldNames[p.Name]; ok {
						if sliceKind == reflect.Struct {
							for j := 0; j < v.Len(); j++ {
								//if the slice is made of structs we encode them

								if err := encodeStruct(v.Index(j).Addr().Interface(), &props, true, val.childStruct); err != nil {
									panic(err)
								}
							}
							continue
						}

						//todo: improve code
						for j := 0; j < v.Len(); j++ {
							sp := datastore.Property{}
							sp.Multiple = true
							sp.Name = p.Name
							sp.NoIndex = true
							//get the element at address j
							sv := v.Index(j).Addr().Elem()
							switch svi := sv.Interface().(type) {
							case time.Time:
								sp.Value = svi
							case appengine.BlobKey:
								sp.Value = svi
							case appengine.GeoPoint:
								sp.Value = svi
							case datastore.ByteString:
								sp.Value = svi
							case *datastore.Key:
								sp.Value = svi
							default:
								switch sv.Kind() {
								case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
									sp.Value = sv.Int()
								case reflect.Bool:
									sp.Value = sv.Bool()
								case reflect.String:
									sp.Value = sv.String()
								case reflect.Float32, reflect.Float64:
									sp.Value = sv.Float()
								}
							}

							props = append(props, sp)
						}
						continue
					}
				}

				//if we have a byteslice:
				p.Multiple = true
				p.NoIndex = true
				p.Value = v.Bytes()
			case reflect.Struct:
				if !v.CanAddr() {
					return nil, fmt.Errorf("datastore: unsupported struct field %s: value is unaddressable", field.Name)
				}
				//if struct, recursively call itself until an error is found
				//as debug, check consistency. we should have a value at i
				if val, ok := model.fieldNames[p.Name]; ok {
					err := encodeStruct(v.Addr().Interface(), &props, false, val.childStruct)
					if err != nil {
						panic(err)
					}
					continue
				}
				return nil, fmt.Errorf("FieldName % s not found in %v for Entity of type %s", p.Name, model.fieldNames, sType)
			}
		}
		props = append(props, p)
	}
	return props, nil
}


func fromPropertyList(modelable modelable, props []datastore.Property) error {
	//get the underlying prototype

	value := reflect.ValueOf(modelable).Elem()
	sType := value.Type()
	model := modelable.getModel()
	pl := propertyLoader{}

	for _, p := range props {
		//if we have a reference we set the key in the corresponding model index
		//to be processed later within datastore transaction

		//we consider a reference only if the model says so.
		//in this way we can mix model. with datastore. package
		pure := pureName(p.Name)
		if field, ok := sType.FieldByName(pure); ok {
			if ref, ok := model.references[field.Index[0]]; ok {
				//cast to key
				if key, ok := p.Value.(*datastore.Key); ok {
					rm := ref.Modelable.getModel()
					rm.Key = key
					continue
				}

				return fmt.Errorf("no struct of type key found for reference %s", pure)
			}
		}

		//load first level values.
		if attr, ok := model.fieldNames[p.Name]; ok {
			err := decodeStruct(reflect.ValueOf(modelable), p, attr, &pl)
			if nil != err {
				return err
			}
			continue
		}
		//if is not in the first level get the first level name
		//firstLevelName := strings.Split(p.Name, ".")[0];
		bname := baseName(p.Name)
		if attr, ok := model.fieldNames[bname]; ok {
			err := decodeStruct(reflect.ValueOf(modelable), p, attr, &pl)
			if nil != err {
				return err
			}
		}
	}
	return nil
}