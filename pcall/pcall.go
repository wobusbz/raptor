package pcall

import (
	"errors"
	"fmt"
	"log"
	"reflect"
	"runtime/debug"
	"strconv"
)

func Pcall0(method reflect.Method, args []reflect.Value) {
	defer func() {
		if rec := recover(); rec != nil {
			stackTrace := debug.Stack()
			stackTraceAsRawStringLiteral := strconv.Quote(string(stackTrace))
			log.Printf("panic - dispatch: methodName=%s panicData=%v stackTrace=%s\n", method.Name, rec, stackTraceAsRawStringLiteral)
		}
	}()
	_ = method.Func.Call(args)
}

func Pcall1(method reflect.Method, args []reflect.Value) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			stackTrace := debug.Stack()
			stackTraceAsRawStringLiteral := strconv.Quote(string(stackTrace))
			if s, ok := rec.(string); ok {
				err = errors.New(s)
			} else {
				err = fmt.Errorf("rpc call internal error - %s: %v", method.Name, rec)
			}
			log.Printf("panic - dispatch: methodName=%s panicData=%v stackTrace=%s\n", method.Name, rec, stackTraceAsRawStringLiteral)
		}
	}()
	r := method.Func.Call(args)
	if len(r) == 1 {
		if !r[0].IsValid() || (r[0].Kind() == reflect.Pointer || r[0].Kind() == reflect.Interface) && r[0].IsNil() {
			return nil
		}
		return r[0].Interface().(error)
	}
	return nil
}

func PcallN(method reflect.Method, args []reflect.Value) (resurt []reflect.Value, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			stackTrace := debug.Stack()
			stackTraceAsRawStringLiteral := strconv.Quote(string(stackTrace))
			if s, ok := rec.(string); ok {
				err = errors.New(s)
			} else {
				err = fmt.Errorf("rpc call internal error - %s: %v", method.Name, rec)
			}
			log.Printf("panic - dispatch: methodName=%s panicData=%v stackTrace=%s\n", method.Name, rec, stackTraceAsRawStringLiteral)
		}
	}()
	r := method.Func.Call(args)
	if len(r) > 2 {
		var xerrs = r[len(r)-1]
		if !xerrs.IsValid() || (xerrs.Kind() == reflect.Pointer || xerrs.Kind() == reflect.Interface) && xerrs.IsNil() {
			return r[:len(r)-1], nil
		}
		return nil, xerrs.Interface().(error)
	}
	return nil, nil
}
