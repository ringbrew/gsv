package decoder

import (
	"fmt"
	"reflect"
)

type fileTypeDecoder struct {
	fieldInfo
	isRepeated bool
}

func (d *fileTypeDecoder) Decode(input *DecodeInput) error {
	fieldValue := GetFieldValue(input.ReqValue, d.parentIndex)
	field := fieldValue.Field(d.index)

	if d.isRepeated {
		return d.fileSliceDecode(input)
	}
	var fileName string
	// file_name > form > fieldName
	for _, tagInfo := range d.tagInfos {
		if tagInfo.Key == fileNameTag {
			fileName = tagInfo.Value
			break
		}
		if tagInfo.Key == formTag {
			fileName = tagInfo.Value
		}
	}
	if len(fileName) == 0 {
		fileName = d.fieldName
	}
	_, file, err := input.Request.FormFile(fileName)
	if err != nil {
		return fmt.Errorf("can not get file '%s', err: %v", fileName, err)
	}
	if field.Kind() == reflect.Ptr {
		t := field.Type()
		var ptrDepth int
		for t.Kind() == reflect.Ptr {
			t = t.Elem()
			ptrDepth++
		}
		v := reflect.New(t).Elem()
		v.Set(reflect.ValueOf(*file))
		field.Set(ReferenceValue(v, ptrDepth))
		return nil
	}

	// Non-pointer elems
	field.Set(reflect.ValueOf(*file))

	return nil
}

func (d *fileTypeDecoder) fileSliceDecode(input *DecodeInput) error {
	fieldValue := GetFieldValue(input.ReqValue, d.parentIndex)
	field := fieldValue.Field(d.index)
	// 如果没值，需要为其建一个值
	if field.Kind() == reflect.Ptr {
		if field.IsNil() {
			nonNilVal, ptrDepth := GetNonNilReferenceValue(field)
			field.Set(ReferenceValue(nonNilVal, ptrDepth))
		}
	}
	var parentPtrDepth int
	for field.Kind() == reflect.Ptr {
		field = field.Elem()
		parentPtrDepth++
	}

	var fileName string
	// file_name > form > fieldName
	for _, tagInfo := range d.tagInfos {
		if tagInfo.Key == fileNameTag {
			fileName = tagInfo.Value
			break
		}
		if tagInfo.Key == formTag {
			fileName = tagInfo.Value
		}
	}
	if len(fileName) == 0 {
		fileName = d.fieldName
	}

	mf := input.Request.MultipartForm
	//multipartForm := mf.Value
	//if err != nil {
	//	return fmt.Errorf("can not get multipartForm info, err: %v", err)
	//}
	files, exist := mf.File[fileName]
	if !exist {
		return fmt.Errorf("the file '%s' is not existed", fileName)
	}

	if field.Kind() == reflect.Array {
		if len(files) != field.Len() {
			return fmt.Errorf("the numbers(%d) of file '%s' does not match the length(%d) of %s", len(files), fileName, field.Len(), field.Type().String())
		}
	} else {
		// slice need creating enough capacity
		field = reflect.MakeSlice(field.Type(), len(files), len(files))
	}

	// handle multiple pointer
	var ptrDepth int
	t := d.fieldType.Elem()
	elemKind := t.Kind()
	for elemKind == reflect.Ptr {
		t = t.Elem()
		elemKind = t.Kind()
		ptrDepth++
	}

	for idx, file := range files {
		v := reflect.New(t).Elem()
		v.Set(reflect.ValueOf(*file))
		field.Index(idx).Set(ReferenceValue(v, ptrDepth))
	}
	fieldValue.Field(d.index).Set(ReferenceValue(field, parentPtrDepth))

	return nil
}

func getMultipartFileDecoder(field reflect.StructField, index int, tagInfos []TagInfo, parentIdx []int, config *DecodeConfig) ([]fieldDecoder, error) {
	fieldType := field.Type
	for field.Type.Kind() == reflect.Ptr {
		fieldType = field.Type.Elem()
	}
	isRepeated := false
	if fieldType.Kind() == reflect.Array || fieldType.Kind() == reflect.Slice {
		isRepeated = true
	}

	return []fieldDecoder{&fileTypeDecoder{
		fieldInfo: fieldInfo{
			index:       index,
			parentIndex: parentIdx,
			fieldName:   field.Name,
			tagInfos:    tagInfos,
			fieldType:   fieldType,
			config:      config,
		},
		isRepeated: isRepeated,
	}}, nil
}
