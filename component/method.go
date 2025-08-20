package component

import (
	"reflect"
	"unicode"
	"unicode/utf8"
)

var (
	typeOfError = reflect.TypeOf((*error)(nil)).Elem()
	typeOfBytes = reflect.TypeOf(([]byte)(nil))
)

func isExported(name string) bool {
	w, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(w)
}

func isExportedOrBuiltinType(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return isExported(t.Name()) || t.PkgPath() == ""
}

func isHandlerMethod(method reflect.Method) bool {
	mt := method.Type
	if method.PkgPath != "" {
		return false
	}
	if mt.NumIn() != 3 {
		return false
	}
	if mt.NumOut() != 1 {
		return false
	}
	if t1 := mt.In(1); t1.Kind() != reflect.Pointer {
		return false
	}

	if (mt.In(2).Kind() != reflect.Pointer && mt.In(2) != typeOfBytes) || mt.Out(0) != typeOfError {
		return false
	}
	return true
}
