package binding

import (
	"fmt"
	inDecoder "github.com/ringbrew/gsv/server/binding/internal/decoder"
	"reflect"
	"time"
)

// BindConfig contains options for default bind behavior.
type BindConfig struct {
	// LooseZeroMode if set to true,
	// the empty string request parameter is bound to the zero value of parameter.
	// NOTE:
	//	The default is false.
	//	Suitable for these parameter types: query/header/cookie/form .
	LooseZeroMode bool
	// DisableDefaultTag is used to add default tags to a field when it has no tag
	// If is false, the field with no tag will be added default tags, for more automated binding. But there may be additional overhead.
	// NOTE:
	// The default is false.
	DisableDefaultTag bool
	// DisableStructFieldResolve is used to generate a separate decoder for a struct.
	// If is false, the 'struct' field will get a single inDecoder.structTypeFieldTextDecoder, and use json.Unmarshal for decode it.
	// It usually used to add json string to query parameter.
	// NOTE:
	// The default is false.
	DisableStructFieldResolve bool
	// EnableDecoderUseNumber is used to call the UseNumber method on the JSON
	// Decoder instance. UseNumber causes the Decoder to unmarshal a number into an
	// interface{} as a Number instead of as a float64.
	// NOTE:
	// The default is false.
	// It is used for BindJSON().
	EnableDecoderUseNumber bool
	// EnableDecoderDisallowUnknownFields is used to call the DisallowUnknownFields method
	// on the JSON Decoder instance. DisallowUnknownFields causes the Decoder to
	// return an error when the destination is a struct and the input contains object
	// keys which do not match any non-ignored, exported fields in the destination.
	// NOTE:
	// The default is false.
	// It is used for BindJSON().
	EnableDecoderDisallowUnknownFields bool
	// TypeUnmarshalFuncs registers customized type unmarshaler.
	// NOTE:
	// time.Time is registered by default
	TypeUnmarshalFuncs map[reflect.Type]inDecoder.CustomizeDecodeFunc
}

func NewBindConfig() *BindConfig {
	return &BindConfig{
		LooseZeroMode:                      false,
		DisableDefaultTag:                  false,
		DisableStructFieldResolve:          false,
		EnableDecoderUseNumber:             false,
		EnableDecoderDisallowUnknownFields: false,
		TypeUnmarshalFuncs:                 make(map[reflect.Type]inDecoder.CustomizeDecodeFunc),
	}
}

// RegTypeUnmarshal registers customized type unmarshaler.
func (config *BindConfig) RegTypeUnmarshal(t reflect.Type, fn inDecoder.CustomizeDecodeFunc) error {
	// check
	switch t.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Float32, reflect.Float64,
		reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8,
		reflect.Uint, reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8:
		return fmt.Errorf("registration type cannot be a basic type")
	case reflect.Ptr:
		return fmt.Errorf("registration type cannot be a pointer type")
	}
	if config.TypeUnmarshalFuncs == nil {
		config.TypeUnmarshalFuncs = make(map[reflect.Type]inDecoder.CustomizeDecodeFunc)
	}
	config.TypeUnmarshalFuncs[t] = fn
	return nil
}

// MustRegTypeUnmarshal registers customized type unmarshaler. It will panic if exist error.
func (config *BindConfig) MustRegTypeUnmarshal(t reflect.Type, fn func(input *inDecoder.DecodeInput, text string) (reflect.Value, error)) {
	err := config.RegTypeUnmarshal(t, fn)
	if err != nil {
		panic(err)
	}
}

func (config *BindConfig) initTypeUnmarshal() {
	config.MustRegTypeUnmarshal(reflect.TypeOf(time.Time{}), func(input *inDecoder.DecodeInput, text string) (reflect.Value, error) {
		if text == "" {
			return reflect.ValueOf(time.Time{}), nil
		}
		t, err := time.Parse(time.RFC3339, text)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(t), nil
	})
}
