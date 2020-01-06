package model

import (
	"cloud.google.com/go/datastore"
	"encoding/gob"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"
)

//Define special reflect.Type
var (
	typeOfGeoPoint  = reflect.TypeOf(datastore.GeoPoint{})
	typeOfTime      = reflect.TypeOf(time.Time{})
	typeOfModel     = reflect.TypeOf(Model{})
	typeOfModelable = reflect.TypeOf((*modelable)(nil)).Elem()
	typeOfStructure = reflect.TypeOf(structure{})
	typeOfPLS       = reflect.TypeOf((*datastore.PropertyLoadSaver)(nil)).Elem()
)

//struct value represent a struct that internally can map other structs
//fieldIndex is the index of the struct
type encodedField struct {
	index       int
	childStruct *encodedStruct
	tag         string
	isExtension bool
	// if true it implements the datastore.PropertyLoadSaver interface
	isPLS bool
}

// todo convert to bitmask?
type encodedStruct struct {
	searchable bool
	// if true the modelable does not get written if zeroed
	skipIfZero    bool
	readonly      bool
	structName    string
	fieldNames    map[string]encodedField
	referencesIdx []int
	extensionsIdx []int
}

func newEncodedStruct(name string) *encodedStruct {
	mp := make(map[string]encodedField)
	ri := make([]int, 0)
	ei := make([]int, 0)
	return &encodedStruct{structName: name, fieldNames: mp, referencesIdx: ri, extensionsIdx: ei}
}

//Keeps track of encoded structs according to their reflect.Type.
//It is used as a cache to avoid to map structs that have been already mapped
var encodedStructsMutex sync.Mutex
var encodedStructs = map[reflect.Type]*encodedStruct{}

func structTypeByName(name string) reflect.Type {
	for k, v := range encodedStructs {
		if v.structName == name {
			return k
		}
	}
	return nil
}

func encodedStructByName(name string) *encodedStruct {
	for _, v := range encodedStructs {
		if v.structName == name {
			return v
		}
	}
	return nil
}

func mapStructure(t reflect.Type, s *encodedStruct) {
	encodedStructsMutex.Lock()
	mapStructureLocked(t, s)
	encodedStructsMutex.Unlock()
}

// checks if field has tag "tag"
// todo: can we do better than a linear search?
func containsTag(tags []string, value string) string {
	for _, v := range tags {
		if v == value {
			return v
		}
	}
	return ""
}

//maps a structure into a linked list representation of its fields.
//It is used to ease the conversion between the Model framework and the datastore
func mapStructureLocked(t reflect.Type, s *encodedStruct) {
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

		if containsTag(tags, tagSkip) != "" {
			continue
		}

		if !s.searchable && containsTag(tags, tagSearch) != "" {
			s.searchable = true
		}

		sName := field.Name
		sValue := encodedField{index: i}
		if fType.Implements(typeOfPLS) {
			sValue.isPLS = true
		}

		switch fType.Kind() {
		case reflect.Interface:
			s.extensionsIdx = append(s.extensionsIdx, i)
			sValue.isExtension = true
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
			fallthrough
		case reflect.Ptr:
			//if we have a pointer we map the value it points to
			fieldElem := fType.Elem()
			if fieldElem.Kind() != reflect.Struct {
				break
			}
			fType = fieldElem
			fallthrough
		case reflect.Struct:
			//we already mapped the struct, skip further computations
			//else we map the other struct
			cs, saved := encodedStructs[fType]
			if saved {
				sValue.childStruct = cs
				sValue.childStruct.structName = sName
			} else {
				sValue.childStruct = newEncodedStruct(sName)
			}

			// add reference properties
			sValue.childStruct.readonly = containsTag(tags, tagReadonly) != ""
			sValue.childStruct.skipIfZero = containsTag(tags, tagZero) != ""
			if reflect.PtrTo(fType).Implements(typeOfModelable) {
				s.referencesIdx = append(s.referencesIdx, i)
			}

			if !saved {
				mapStructureLocked(fType, sValue.childStruct)
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			fallthrough
		case reflect.Bool:
			fallthrough
		case reflect.String:
			fallthrough
		case reflect.Float32, reflect.Float64:
			fallthrough
		default:
			break
		}

		s.fieldNames[sName] = sValue
	}
	encodedStructs[t] = s

	// once the struct has been mapped
	// register it to the gob for memcache decoding
	obj := reflect.New(t).Interface()
	gob.Register(obj)
}

func encodeStruct(name string, s interface{}, props *[]datastore.Property, multiple bool, codec *encodedStruct) error {
	value := reflect.ValueOf(s).Elem()
	sType := value.Type()

	for i := 0; i < sType.NumField(); i++ {
		field := sType.Field(i)

		if field.Type == typeOfModel {
			continue
		}

		if field.Tag.Get("datastore") == "-" {
			continue
		}

		if field.Tag.Get("model") == "-" {
			continue
		}

		v := value.FieldByName(field.Name)
		p := &datastore.Property{}

		if multiple {
			p.NoIndex = true
		}

		p.Name = referenceName(name, field.Name)
		switch x := v.Interface().(type) {
		case time.Time:
			p.Value = x
		case datastore.GeoPoint:
			p.Value = x
		case []byte:
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
				if v.Type().Elem().Kind() != reflect.Uint8 {
					if val, ok := codec.fieldNames[field.Name]; ok {
						for j := 0; j < v.Len(); j++ {
							if err := encodeStruct(val.childStruct.structName, v.Index(j).Addr().Interface(), props, true, val.childStruct); err != nil {
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
					return fmt.Errorf("datastore: unsupported struct field %s for entity of type %s: value %v is unaddressable", p.Name, sType, v)
				}

				if val, ok := codec.fieldNames[p.Name]; ok {
					if nil != val.childStruct {
						if err := encodeStruct(val.childStruct.structName, v.Addr().Interface(), props, multiple, val.childStruct); err != nil {
							panic(err)
						}
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

func isZero(m interface{}) bool {
	elem := reflect.Indirect(reflect.ValueOf(m))
	fields := elem.NumField()
	for i := 0; i < fields; i++ {
		field := elem.Field(i)
		if field.Type() == typeOfModel {
			continue
		}
		switch field.Kind() {
		case reflect.Struct:
			return isZero(field.Interface())
		case reflect.Ptr:
			if !field.IsNil() {
				return false
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if field.Int() != 0 {
				return false
			}
		case reflect.Bool:
			if field.Bool() {
				return false
			}
		case reflect.String:
			if field.String() != "" {
				return false
			}
		case reflect.Float32, reflect.Float64:
			if field.Float() != 0 {
				return false
			}
		case reflect.Slice, reflect.Array, reflect.Map, reflect.Chan:
			if field.Len() > 0 {
				return false
			}
		}
	}
	return true
}

type propertyLoader struct {
	mem map[string]int
}

func decodeStruct(s reflect.Value, p datastore.Property, encodedField encodedField, l *propertyLoader) error {
	interf := s
	if s.Kind() == reflect.Ptr || s.Kind() == reflect.Interface {
		interf = s.Elem()
	}
	//todo::handle slice exception case where slice of slices

	//get the field we are decoding
	field := interf.Field(encodedField.index)
	switch field.Kind() {
	case reflect.Interface:
		if !isValidExtension(field) {
			msg := fmt.Sprintf("invalid interface type to load into. Admitted only ptr to struct: found %q type at index %d", field.Elem().Type().Name(), encodedField.index)
			panic(msg)
		}

		typ := field.Elem().Elem().Type()
		es, ok := encodedStructs[typ]
		if !ok {
			return fmt.Errorf("struct of type %q has not been mapped. Can't load into field at index %d", typ, encodedField.index)
		}

		name := childName(p.Name)
		if attr, ok := es.fieldNames[name]; ok {
			if err := decodeStruct(field.Elem(), p, attr, l); err != nil {
				return err
			}
		}
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
			x, ok := p.Value.(datastore.GeoPoint)
			if !ok && p.Value != nil {
				return errors.New("error - invalid geoPoint type")
			}
			field.Set(reflect.ValueOf(x))
		default:

			//instantiate a new struct of the type of the field v
			//get the encoded field for the attr of the struct with name == p.Name
			if attr, ok := encodedField.childStruct.fieldNames[p.Name]; ok {
				if err := decodeStruct(field.Addr(), p, attr, l); err != nil {
					return err
				}
			}
			//else go down one level
			cName := childName(p.Name)
			if attr, ok := encodedField.childStruct.fieldNames[cName]; ok {
				if err := decodeStruct(field.Addr(), p, attr, l); err != nil {
					return err
				}
			}
			return nil
		}
	case reflect.Slice:
		sliceKind := field.Type().Elem().Kind()

		x, ok := p.Value.([]byte)
		if !ok {
			if y, yok := p.Value.([]byte); yok {
				x, ok = y, true
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
					if err := decodeStruct(field.Index(index), p, attr, l); err != nil {
						return err
					}
				}

				cName := childName(p.Name)
				if attr, ok := encodedField.childStruct.fieldNames[cName]; ok {
					if err := decodeStruct(field.Index(index), p, attr, l); err != nil {
						return err
					}
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

		if err := decodeField(field, p); err != nil {
			return err
		}
	}

	return nil
}

// todo define errors
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
		case []byte:
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

func referenceName(parentName string, refName string) string {
	if parentName == "" {
		return refName
	}
	return fmt.Sprintf("%s.%s", parentName, refName)
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
		return fullName[lastIndex+1:]
	}
	return fullName
}

func childName(fullName string) string {
	firstIndex := strings.Index(fullName, valSeparator)
	if firstIndex > 0 {
		return fullName[firstIndex+1:]
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

		tags := strings.Split(field.Tag.Get(tagDomain), ",")

		if containsTag(tags, tagSkip) != "" {
			continue
		}

		p := datastore.Property{}

		if containsTag(tags, tagNoindex) != "" {
			p.NoIndex = true
		}

		p.Name = field.Name

		if ref := model.referenceAtIndex(i); ref != nil {
			rm := ref.Modelable.getModel()
			p.Value = rm.Key
			props = append(props, p)
			continue
		}
		v := value.Field(i)
		switch x := v.Interface().(type) {
		case time.Time:
			p.Value = x
		case datastore.GeoPoint:
			p.Value = x
		case []byte:
			p.Value = x
		case *datastore.Key:
			p.Value = x
		case datastore.PropertyLoadSaver:
			eprops, err := x.Save()
			if err != nil {
				return nil, err
			}
			props = append(props, eprops...)
			continue
		default:
			switch v.Kind() {
			case reflect.Interface:
				// if valid interface, treat it like an extension
				if v.IsNil() {
					continue
				}

				if !isValidExtension(v) {
					msg := fmt.Sprintf("only ptr to struct are admitted as interface types. %q type found at index %d", v.Elem().Type(), i)
					panic(msg)
				}

				typ := v.Elem().Elem().Type()
				es, ok := encodedStructs[typ]
				if !ok {
					msg := fmt.Sprintf("struct of type %q has not been mapped. Can't save interface at index %d", typ, i)
					panic(msg)
				}

				p.Name = makeExtensionTypeName(p.Name)
				p.Value = v.Elem().Type().Elem().Name()
				props = append(props, p)

				err := encodeStruct(field.Name, v.Elem().Interface(), &props, false, es)
				if err != nil {
					panic(err)
				}
				continue
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

								if err := encodeStruct(val.childStruct.structName, v.Index(j).Addr().Interface(), &props, true, val.childStruct); err != nil {
									panic(err)
								}
							}
							continue
						}

						//todo: improve code
						for j := 0; j < v.Len(); j++ {
							sp := datastore.Property{}
							sp.Name = p.Name
							sp.NoIndex = true
							//get the element at address j
							sv := v.Index(j).Addr().Elem()
							switch svi := sv.Interface().(type) {
							case time.Time:
								sp.Value = svi
							case datastore.GeoPoint:
								sp.Value = svi
							case []byte:
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

				p.NoIndex = true
				p.Value = v.Bytes()
			case reflect.Struct:
				if !v.CanAddr() {
					return nil, fmt.Errorf("datastore: unsupported struct field %s: value is unaddressable", field.Name)
				}
				//if struct, recursively call itself until an error is found
				//as debug, check consistency. we should have a value at i
				if val, ok := model.fieldNames[p.Name]; ok {
					err := encodeStruct(val.childStruct.structName, v.Addr().Interface(), &props, false, val.childStruct)
					if err != nil {
						panic(err)
					}
					continue
				}
				return nil, fmt.Errorf("FieldName %s not found in %v for Entity of type %s", p.Name, model.fieldNames, sType)
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
			if ref := model.referenceAtIndex(field.Index[0]); ref != nil {
				//cast to key
				if key, ok := p.Value.(*datastore.Key); ok || p.Value == nil {
					rm := ref.Modelable.getModel()
					rm.Key = key
					continue
				}

				return fmt.Errorf("no struct of type key found for reference %s", pure)
			}
		}

		//if is not in the first level get the first level name
		//firstLevelName := strings.Split(p.Name, ".")[0];
		bname := baseName(p.Name)
		if attr, ok := model.fieldNames[bname]; ok {

			val := reflect.ValueOf(modelable)

			if attr.isExtension {
				// if the value of the extension is currently nil, create it
				if field := val.Elem().Field(attr.index); field.IsNil() {
					extype := findExtensionType(bname, props)
					if extype == nil {
						return fmt.Errorf("no valid type for Extension field %s", bname)
					}

					obj := reflect.New(extype)
					field.Set(obj)
				}
			}

			err := decodeStruct(val, p, attr, &pl)
			if nil != err {
				return err
			}
			continue
		}
	}

	// handle PLS
	for k, v := range model.fieldNames {
		if v.isPLS {
			field := reflect.ValueOf(modelable).Elem().FieldByName(k)
			obj := reflect.New(field.Type().Elem())
			field.Set(obj)
			pls := field.Interface().(datastore.PropertyLoadSaver)
			if err := pls.Load(props); err != nil {
				panic(err)
			}
		}
	}

	return nil
}

func findExtensionType(ext string, props []datastore.Property) reflect.Type {
	needle := makeExtensionTypeName(ext)
	for _, v := range props {
		if v.Name == needle {
			return structTypeByName(v.Value.(string))
		}
	}
	return nil
}
