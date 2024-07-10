package binding

import (
	"fmt"
	"reflect"
	"unsafe"
)

func valueAndTypeID(v interface{}) (reflect.Value, uintptr) {
	header := (*emptyInterface)(unsafe.Pointer(&v))
	rv := reflect.ValueOf(v)
	return rv, header.typeID
}

type emptyInterface struct {
	typeID  uintptr
	dataPtr unsafe.Pointer
}

func checkPointer(rv reflect.Value) error {
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("receiver must be a non-nil pointer")
	}
	return nil
}

func dereferPointer(rv reflect.Value) reflect.Type {
	rt := rv.Type()
	for rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	return rt
}
