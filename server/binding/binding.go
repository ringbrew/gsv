package binding

import (
	"encoding/json"
	"fmt"
	"github.com/ringbrew/gsv/server/binding/common"
	"github.com/ringbrew/gsv/server/binding/consts"
	inDecoder "github.com/ringbrew/gsv/server/binding/internal/decoder"
	"github.com/ringbrew/gsv/server/binding/param"
	"google.golang.org/protobuf/proto"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

type Binder interface {
	Name() string
	Bind(*http.Request, interface{}, param.Params) error
	BindAndValidate(*http.Request, interface{}, param.Params) error
	BindQuery(*http.Request, interface{}) error
	BindHeader(*http.Request, interface{}) error
	BindPath(*http.Request, interface{}, param.Params) error
	BindForm(*http.Request, interface{}) error
	BindJSON(*http.Request, interface{}) error
	BindProtobuf(*http.Request, interface{}) error
}

const (
	queryTag           = "query"
	headerTag          = "header"
	formTag            = "form"
	pathTag            = "path"
	defaultValidateTag = "vd"
)

type decoderInfo struct {
	decoder inDecoder.Decoder
}

var defaultBind = NewDefaultBinder(nil)

func DefaultBinder() Binder {
	return defaultBind
}

type defaultBinder struct {
	config             *BindConfig
	decoderCache       sync.Map
	queryDecoderCache  sync.Map
	formDecoderCache   sync.Map
	headerDecoderCache sync.Map
	pathDecoderCache   sync.Map
}

func NewDefaultBinder(config *BindConfig) Binder {
	if config == nil {
		bindConfig := NewBindConfig()
		bindConfig.initTypeUnmarshal()
		return &defaultBinder{
			config: bindConfig,
		}
	}
	config.initTypeUnmarshal()

	return &defaultBinder{
		config: config,
	}
}

func (b *defaultBinder) tagCache(tag string) *sync.Map {
	switch tag {
	case queryTag:
		return &b.queryDecoderCache
	case headerTag:
		return &b.headerDecoderCache
	case formTag:
		return &b.formDecoderCache
	case pathTag:
		return &b.pathDecoderCache
	default:
		return &b.decoderCache
	}
}

func (b *defaultBinder) bindTag(req *http.Request, v interface{}, params param.Params, tag string) error {
	rv, typeID := valueAndTypeID(v)
	if err := checkPointer(rv); err != nil {
		return err
	}
	rt := dereferPointer(rv)
	if rt.Kind() != reflect.Struct {
		return b.bindNonStruct(req, v)
	}

	if len(tag) == 0 {
		err := b.preBindBody(req, v)
		if err != nil {
			return fmt.Errorf("bind body failed, err=%v", err)
		}
	}

	input := inDecoder.NewDecodeInput(req, params, rv.Elem())

	cache := b.tagCache(tag)
	cached, ok := cache.Load(typeID)
	if ok {
		// cached fieldDecoder, fast path
		decoder := cached.(decoderInfo)
		return decoder.decoder(input)
	}

	decodeConfig := &inDecoder.DecodeConfig{
		LooseZeroMode:                      b.config.LooseZeroMode,
		DisableDefaultTag:                  b.config.DisableDefaultTag,
		DisableStructFieldResolve:          b.config.DisableStructFieldResolve,
		EnableDecoderUseNumber:             b.config.EnableDecoderUseNumber,
		EnableDecoderDisallowUnknownFields: b.config.EnableDecoderDisallowUnknownFields,
		TypeUnmarshalFuncs:                 b.config.TypeUnmarshalFuncs,
	}

	decoder, err := inDecoder.GetReqDecoder(rv.Type(), tag, decodeConfig)
	if err != nil {
		return err
	}

	cache.Store(typeID, decoderInfo{decoder: decoder})
	return decoder(input)
}

func (b *defaultBinder) bindTagWithValidate(req *http.Request, v interface{}, params param.Params, tag string) error {
	rv, typeID := valueAndTypeID(v)
	if err := checkPointer(rv); err != nil {
		return err
	}
	rt := dereferPointer(rv)
	if rt.Kind() != reflect.Struct {
		return b.bindNonStruct(req, v)
	}

	input := inDecoder.NewDecodeInput(req, params, rv.Elem())

	err := b.preBindBody(req, v)
	if err != nil {
		return fmt.Errorf("bind body failed, err=%v", err)
	}
	cache := b.tagCache(tag)
	cached, ok := cache.Load(typeID)
	if ok {
		// cached fieldDecoder, fast path
		decoder := cached.(decoderInfo)
		err = decoder.decoder(input)
		if err != nil {
			return err
		}
		return err
	}

	decodeConfig := &inDecoder.DecodeConfig{
		LooseZeroMode:                      b.config.LooseZeroMode,
		DisableDefaultTag:                  b.config.DisableDefaultTag,
		DisableStructFieldResolve:          b.config.DisableStructFieldResolve,
		EnableDecoderUseNumber:             b.config.EnableDecoderUseNumber,
		EnableDecoderDisallowUnknownFields: b.config.EnableDecoderDisallowUnknownFields,
		TypeUnmarshalFuncs:                 b.config.TypeUnmarshalFuncs,
	}
	decoder, err := inDecoder.GetReqDecoder(rv.Type(), tag, decodeConfig)
	if err != nil {
		return err
	}

	cache.Store(typeID, decoderInfo{decoder: decoder})
	err = decoder(input)
	if err != nil {
		return err
	}

	return err
}

func (b *defaultBinder) BindQuery(req *http.Request, v interface{}) error {
	return b.bindTag(req, v, nil, queryTag)
}

func (b *defaultBinder) BindHeader(req *http.Request, v interface{}) error {
	return b.bindTag(req, v, nil, headerTag)
}

func (b *defaultBinder) BindPath(req *http.Request, v interface{}, params param.Params) error {
	return b.bindTag(req, v, params, pathTag)
}

func (b *defaultBinder) BindForm(req *http.Request, v interface{}) error {
	return b.bindTag(req, v, nil, formTag)
}

func (b *defaultBinder) BindJSON(req *http.Request, v interface{}) error {
	return b.decodeJSON(req.Body, v)
}

func (b *defaultBinder) decodeJSON(r io.Reader, obj interface{}) error {
	decoder := json.NewDecoder(r)
	if b.config.EnableDecoderUseNumber {
		decoder.UseNumber()
	}
	if b.config.EnableDecoderDisallowUnknownFields {
		decoder.DisallowUnknownFields()
	}

	return decoder.Decode(obj)
}

func (b *defaultBinder) BindProtobuf(req *http.Request, v interface{}) error {
	msg, ok := v.(proto.Message)
	if !ok {
		return fmt.Errorf("%s does not implement 'proto.Message'", v)
	}

	bodyData, err := io.ReadAll(req.Body)
	if err != nil {
		return err
	}

	return proto.Unmarshal(bodyData, msg)
}

func (b *defaultBinder) Name() string {
	return "hertz"
}

func (b *defaultBinder) BindAndValidate(req *http.Request, v interface{}, params param.Params) error {
	return b.bindTagWithValidate(req, v, params, "")
}

func (b *defaultBinder) Bind(req *http.Request, v interface{}, params param.Params) error {
	return b.bindTag(req, v, params, "")
}

// best effort binding
func (b *defaultBinder) preBindBody(req *http.Request, v interface{}) error {
	if val := req.Header.Get(consts.HeaderContentLength); val == "" {
		return nil
	} else {
		if cl, err := strconv.Atoi(val); err != nil {
			return nil
		} else if cl <= 0 {
			return nil
		}
	}
	ct := req.Header.Get(consts.HeaderContentType)
	bodyData, err := io.ReadAll(req.Body)
	if err != nil {
		return err
	}

	switch strings.ToLower(common.FilterContentType(ct)) {
	case consts.MIMEApplicationJSON:
		return json.Unmarshal(bodyData, v)
	case consts.MIMEPROTOBUF:
		msg, ok := v.(proto.Message)
		if !ok {
			return fmt.Errorf("%s can not implement 'proto.Message'", v)
		}
		return proto.Unmarshal(bodyData, msg)
	default:
		return nil
	}
}

func (b *defaultBinder) bindNonStruct(req *http.Request, v interface{}) (err error) {
	ct := req.Header.Get(consts.HeaderContentType)
	bodyData, err := io.ReadAll(req.Body)
	if err != nil {
		return err
	}
	switch strings.ToLower(common.FilterContentType(ct)) {
	case consts.MIMEApplicationJSON:
		err = json.Unmarshal(bodyData, v)
	case consts.MIMEPROTOBUF:
		msg, ok := v.(proto.Message)
		if !ok {
			return fmt.Errorf("%s can not implement 'proto.Message'", v)
		}
		err = proto.Unmarshal(bodyData, msg)
	case consts.MIMEMultipartPOSTForm:
		form := make(url.Values)
		for k, vv := range req.MultipartForm.Value {
			for _, vvv := range vv {
				form.Add(k, vvv)
			}
		}

		tmp, _ := json.Marshal(form)
		err = json.Unmarshal(tmp, v)
	case consts.MIMEApplicationHTMLForm:
		var form url.Values = req.Form
		tmp, _ := json.Marshal(form)
		err = json.Unmarshal(tmp, v)
	default:
		// using query to decode
		var query url.Values = req.URL.Query()
		tmp, _ := json.Marshal(query)
		err = json.Unmarshal(tmp, v)
	}
	return
}
