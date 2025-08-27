package component

import (
	"game/session"
	"reflect"
	"unicode"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"
)

var (
	typeOfSession  = reflect.TypeOf((*session.Session)(nil)).Elem()
	typeOfError    = reflect.TypeOf((*error)(nil)).Elem()
	typeOfMessages = reflect.TypeOf((*proto.Message)(nil)).Elem()
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
	if t1 := mt.In(1); t1.Kind() != reflect.Interface && !t1.Implements(typeOfSession) {
		return false
	}
	if !mt.In(2).Implements(typeOfMessages) {
		return false
	}

	return true
}
