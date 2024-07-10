package decoder

import (
	"fmt"
	"github.com/ringbrew/gsv/server/binding/consts"
	"github.com/ringbrew/gsv/server/binding/param"
	"mime/multipart"
	"net/http"
	"reflect"
	"sync"
)

type DecodeInput struct {
	Request  *http.Request
	Params   param.Params // url path params.
	ReqValue reflect.Value

	sync.RWMutex
}

func NewDecodeInput(req *http.Request, params param.Params, v reflect.Value) *DecodeInput {
	return &DecodeInput{
		Request:  req,
		Params:   params,
		ReqValue: v,
	}
}

func (di *DecodeInput) ContentType() string {
	return di.Request.Header.Get(consts.HeaderContentType)
}

func (di *DecodeInput) ContentLength() int64 {
	return di.Request.ContentLength
}

func (di *DecodeInput) Body() []byte {
	return nil
}

type fieldInfo struct {
	index       int
	parentIndex []int
	fieldName   string
	tagInfos    []TagInfo
	fieldType   reflect.Type
	config      *DecodeConfig
}

type fieldDecoder interface {
	Decode(input *DecodeInput) error
}

type Decoder func(input *DecodeInput) error

type DecodeConfig struct {
	LooseZeroMode                      bool
	DisableDefaultTag                  bool
	DisableStructFieldResolve          bool
	EnableDecoderUseNumber             bool
	EnableDecoderDisallowUnknownFields bool
	ValidateTag                        string
	TypeUnmarshalFuncs                 map[reflect.Type]CustomizeDecodeFunc
}

func GetReqDecoder(rt reflect.Type, byTag string, config *DecodeConfig) (Decoder, error) {
	var decoders []fieldDecoder

	el := rt.Elem()
	if el.Kind() != reflect.Struct {
		return nil, fmt.Errorf("unsupported \"%s\" type binding", rt.String())
	}

	for i := 0; i < el.NumField(); i++ {
		if el.Field(i).PkgPath != "" && !el.Field(i).Anonymous {
			// ignore unexported field
			continue
		}

		dec, err := getFieldDecoder(parentInfos{[]reflect.Type{el}, []int{}, ""}, el.Field(i), i, byTag, config)
		if err != nil {
			return nil, err
		}

		if dec != nil {
			decoders = append(decoders, dec...)
		}
	}

	return func(input *DecodeInput) error {
		for _, decoder := range decoders {
			err := decoder.Decode(input)
			if err != nil {
				return err
			}
		}

		return nil
	}, nil
}

type parentInfos struct {
	Types    []reflect.Type
	Indexes  []int
	JSONName string
}

func getFieldDecoder(pInfo parentInfos, field reflect.StructField, index int, byTag string, config *DecodeConfig) ([]fieldDecoder, error) {
	for field.Type.Kind() == reflect.Ptr {
		field.Type = field.Type.Elem()
	}
	// skip anonymous definitions, like:
	// type A struct {
	// 		string
	// }
	if field.Type.Kind() != reflect.Struct && field.Anonymous {
		return nil, nil
	}

	// JSONName is like 'a.b.c' for 'required validate'
	fieldTagInfos, newParentJSONName := lookupFieldTags(field, pInfo.JSONName, config)
	if len(fieldTagInfos) == 0 && !config.DisableDefaultTag {
		fieldTagInfos, newParentJSONName = getDefaultFieldTags(field, pInfo.JSONName)
	}
	if len(byTag) != 0 {
		fieldTagInfos = getFieldTagInfoByTag(field, byTag)
	}

	// customized type decoder has the highest priority
	if customizedFunc, exist := config.TypeUnmarshalFuncs[field.Type]; exist {
		dec, err := getCustomizedFieldDecoder(field, index, fieldTagInfos, pInfo.Indexes, customizedFunc, config)
		return dec, err
	}

	// slice/array field decoder
	if field.Type.Kind() == reflect.Slice || field.Type.Kind() == reflect.Array {
		dec, err := getSliceFieldDecoder(field, index, fieldTagInfos, pInfo.Indexes, config)
		return dec, err
	}

	// map filed decoder
	if field.Type.Kind() == reflect.Map {
		dec, err := getMapTypeTextDecoder(field, index, fieldTagInfos, pInfo.Indexes, config)
		return dec, err
	}

	// struct field will be resolved recursively
	if field.Type.Kind() == reflect.Struct {
		var decoders []fieldDecoder
		el := field.Type
		// todo: more built-in common struct binding, ex. time...
		switch el {
		case reflect.TypeOf(multipart.FileHeader{}): // file binding
			dec, err := getMultipartFileDecoder(field, index, fieldTagInfos, pInfo.Indexes, config)
			return dec, err
		}
		if !config.DisableStructFieldResolve { // decode struct type separately
			structFieldDecoder, err := getStructTypeFieldDecoder(field, index, fieldTagInfos, pInfo.Indexes, config)
			if err != nil {
				return nil, err
			}
			if structFieldDecoder != nil {
				decoders = append(decoders, structFieldDecoder...)
			}
		}

		// prevent infinite recursion when struct field with the same name as a struct
		if hasSameType(pInfo.Types, el) {
			return decoders, nil
		}

		pIdx := pInfo.Indexes
		for i := 0; i < el.NumField(); i++ {
			if el.Field(i).PkgPath != "" && !el.Field(i).Anonymous {
				// ignore unexported field
				continue
			}
			var idxes []int
			if len(pInfo.Indexes) > 0 {
				idxes = append(idxes, pIdx...)
			}
			idxes = append(idxes, index)
			pInfo.Indexes = idxes
			pInfo.Types = append(pInfo.Types, el)
			pInfo.JSONName = newParentJSONName
			dec, err := getFieldDecoder(pInfo, el.Field(i), i, byTag, config)
			if err != nil {
				return nil, err
			}
			if dec != nil {
				decoders = append(decoders, dec...)
			}
		}

		return decoders, nil
	}

	// base type decoder
	dec, err := getBaseTypeTextDecoder(field, index, fieldTagInfos, pInfo.Indexes, config)
	return dec, err
}

// hasSameType determine if the same type is present in the parent-child relationship
func hasSameType(pts []reflect.Type, ft reflect.Type) bool {
	for _, pt := range pts {
		if reflect.DeepEqual(getElemType(pt), getElemType(ft)) {
			return true
		}
	}
	return false
}
