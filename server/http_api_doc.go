package server

import (
	"reflect"
	"strings"
)

type DocService struct {
	Key  string
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
	Module          string
	Action          string
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
	structLink := make(map[string]int)

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
		for i := range list {
			structLink[list[i].Name()]++
		}

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
					rt := realT(fieldInfo.Type)
					list = append(list, rt)
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

	// mark root.
	if len(result) > 0 {
		result[0].Root = true
	}

	// process anonymous.
	tMap := make(map[string]Struct)
	for i := range result {
		tMap[result[i].Name] = result[i]
	}
	var extraField func(input Struct) []Field
	extraField = func(input Struct) []Field {
		rs := make([]Field, 0)
		for i := range input.Field {
			if input.Field[i].Anonymous {
				si := tMap[input.Field[i].Type.Name]
				structLink[input.Field[i].Type.Name]--
				asf := extraField(si)
				rs = append(rs, asf...)
			} else {
				rs = append(rs, input.Field[i])
			}
		}
		return rs
	}

	for i := range result {
		result[i].Field = extraField(result[i])
	}

	final := make([]Struct, 0, len(result))
	for i := range result {
		if structLink[result[i].Name] > 0 {
			final = append(final, result[i])
		}
	}

	return final
}
