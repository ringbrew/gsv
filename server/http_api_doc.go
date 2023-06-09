package server

import (
	"reflect"
	"strings"
)

type DocService struct {
	Name string
	Api  []DocApi
}

type DocApi struct {
	Name            string
	Path            string
	Method          string
	ContentType     string
	Request         []Struct
	Response        []Struct
	RequestExample  string
	ResponseExample string
}

type Struct struct {
	Name  string
	Field []Field
	Root  bool
}

type Field struct {
	Name      string
	Type      FieldType
	Remark    string
	Required  bool
	Anonymous bool
}

type FieldType struct {
	Category FieldTypeCategory
	Name     string
	Items    *FieldType
}

type FieldTypeCategory int

const (
	FieldTypeCategoryInvalid FieldTypeCategory = iota
	FieldTypeCategoryBasic
	FieldTypeCategoryObject
	FieldTypeCategoryArray
)

type TypeItem struct {
	Name string
	Type string
}

func structInfo(input reflect.Type) []Struct {
	list := make([]reflect.Type, 0, 1)
	list = append(list, input)
	result := make([]Struct, 0, 1)
	set := make(map[string]struct{})

	realT := func(rt reflect.Type) reflect.Type {
		for rt.Kind() == reflect.Ptr || rt.Kind() == reflect.Slice {
			rt = rt.Elem() // use Elem to get the pointed-to-type
		}
		return rt
	}

	var realFieldType func(rt reflect.Type) FieldType
	realFieldType = func(rt reflect.Type) FieldType {
		ft := FieldType{}

		for rt.Kind() == reflect.Ptr || rt.Kind() == reflect.Slice {
			if rt.Kind() == reflect.Slice {
				ft.Category = FieldTypeCategoryArray
				ft.Name = "array"
				rt = rt.Elem() // use Elem to get the pointed-to-type
				embed := realFieldType(rt)
				ft.Items = &embed
			} else {
				rt = rt.Elem()
			}
		}

		if ft.Category != FieldTypeCategoryArray {
			if rt.Kind() == reflect.Struct {
				ft.Category = FieldTypeCategoryObject
				ft.Name = rt.Name()
			} else {
				ft.Category = FieldTypeCategoryBasic
				ft.Name = rt.Name()
			}
		}

		return ft
	}

	for len(list) > 0 {
		process := append([]reflect.Type{}, list...)
		list = make([]reflect.Type, 0)

		for _, t := range process {
			if t.Kind() != reflect.Struct {
				continue
			}

			if _, exist := set[t.Name()]; !exist {
				set[t.Name()] = struct{}{}
			} else {
				continue
			}

			curr := Struct{
				Name: t.Name(),
			}

			fieldNum := t.NumField()
			for i := 0; i < fieldNum; i++ {
				fieldInfo := t.Field(i)

				if len(fieldInfo.Name) > 0 {
					r := []rune(fieldInfo.Name)

					s := string(r[0])

					if strings.ToLower(s) == s {
						continue
					}
				}

				if fieldInfo.Type.Kind() == reflect.Struct || fieldInfo.Type.Kind() == reflect.Ptr || fieldInfo.Type.Kind() == reflect.Slice {
					list = append(list, realT(fieldInfo.Type))
				}

				name := fieldInfo.Name

				if jt := fieldInfo.Tag.Get("json"); jt != "" {
					name = strings.Split(jt, ",")[0]
				}

				f := Field{
					Name:      name,
					Type:      realFieldType(fieldInfo.Type),
					Remark:    fieldInfo.Tag.Get("remark"),
					Anonymous: fieldInfo.Anonymous,
				}

				validate := fieldInfo.Tag.Get("validate")

				if strings.Contains(validate, "required") {
					f.Required = true
				}

				curr.Field = append(curr.Field, f)
			}

			result = append(result, curr)
		}

	}

	if len(result) > 0 {
		result[0].Root = true
	}

	return result
}
