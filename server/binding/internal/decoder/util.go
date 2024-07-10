package decoder

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

const (
	specialChar = "\x07"
)

// toDefaultValue will preprocess the default value and transfer it to be standard format
func toDefaultValue(typ reflect.Type, defaultValue string) string {
	switch typ.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map, reflect.Struct:
		// escape single quote and double quote, replace single quote with double quote
		defaultValue = strings.Replace(defaultValue, `"`, `\"`, -1)
		defaultValue = strings.Replace(defaultValue, `\'`, specialChar, -1)
		defaultValue = strings.Replace(defaultValue, `'`, `"`, -1)
		defaultValue = strings.Replace(defaultValue, specialChar, `'`, -1)
	}
	return defaultValue
}

// stringToValue is used to dynamically create reflect.Value for 'text'
func stringToValue(elemType reflect.Type, text string, input *DecodeInput, config *DecodeConfig) (v reflect.Value, err error) {
	v = reflect.New(elemType).Elem()
	if customizedFunc, exist := config.TypeUnmarshalFuncs[elemType]; exist {
		val, err := customizedFunc(input, text)
		if err != nil {
			return reflect.Value{}, err
		}
		return val, nil
	}
	switch elemType.Kind() {
	case reflect.Struct:
		err = json.Unmarshal([]byte(text), v.Addr().Interface())
	case reflect.Map:
		err = json.Unmarshal([]byte(text), v.Addr().Interface())
	case reflect.Array, reflect.Slice:
		// do nothing
	default:
		decoder, err := SelectTextDecoder(elemType)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("unsupported type %s for slice/array", elemType.String())
		}
		err = decoder.UnmarshalString(text, v, config.LooseZeroMode)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("unable to decode '%s' as %s: %w", text, elemType.String(), err)
		}
	}

	return v, err
}
