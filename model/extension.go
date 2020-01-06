package model

import (
	"fmt"
	"reflect"
)

const typeAppendix = "__ptrType"

// extension must always be embedded into a first level model
// when converted to property list, extension adds a custom meta data as Ext.__type

func makeExtensionTypeName(base string) string {
	return fmt.Sprintf("%s%s%s", base, valSeparator, typeAppendix)
}

func isValidExtension(v reflect.Value) bool {
	isPtr := v.Elem().Kind() == reflect.Ptr
	isStruct := v.Elem().Elem().Kind() == reflect.Struct
	return isPtr && isStruct
}
