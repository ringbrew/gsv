package decoder

import (
	"reflect"
)

// ReferenceValue convert T to *T, the ptrDepth is the count of '*'.
func ReferenceValue(v reflect.Value, ptrDepth int) reflect.Value {
	switch {
	case ptrDepth > 0:
		for ; ptrDepth > 0; ptrDepth-- {
			vv := reflect.New(v.Type())
			vv.Elem().Set(v)
			v = vv
		}
	case ptrDepth < 0:
		for ; ptrDepth < 0 && v.Kind() == reflect.Ptr; ptrDepth++ {
			v = v.Elem()
		}
	}
	return v
}

func GetNonNilReferenceValue(v reflect.Value) (reflect.Value, int) {
	var ptrDepth int
	t := v.Type()
	elemKind := t.Kind()
	for elemKind == reflect.Ptr {
		t = t.Elem()
		elemKind = t.Kind()
		ptrDepth++
	}
	val := reflect.New(t).Elem()
	return val, ptrDepth
}

func GetFieldValue(reqValue reflect.Value, parentIndex []int) reflect.Value {
	// reqValue -> (***bar)(nil) need new a default
	if reqValue.Kind() == reflect.Ptr && reqValue.IsNil() {
		nonNilVal, ptrDepth := GetNonNilReferenceValue(reqValue)
		reqValue = ReferenceValue(nonNilVal, ptrDepth)
	}
	for _, idx := range parentIndex {
		if reqValue.Kind() == reflect.Ptr && reqValue.IsNil() {
			nonNilVal, ptrDepth := GetNonNilReferenceValue(reqValue)
			reqValue.Set(ReferenceValue(nonNilVal, ptrDepth))
		}
		for reqValue.Kind() == reflect.Ptr {
			reqValue = reqValue.Elem()
		}
		reqValue = reqValue.Field(idx)
	}

	// It is possible that the parent struct is also a pointer,
	// so need to create a non-nil reflect.Value for it at runtime.
	for reqValue.Kind() == reflect.Ptr {
		if reqValue.IsNil() {
			nonNilVal, ptrDepth := GetNonNilReferenceValue(reqValue)
			reqValue.Set(ReferenceValue(nonNilVal, ptrDepth))
		}
		reqValue = reqValue.Elem()
	}

	return reqValue
}

func getElemType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	return t
}
