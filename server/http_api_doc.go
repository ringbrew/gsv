package server

import (
	"reflect"
	"strings"
)

type HttpDocService struct {
	Name string
	Api  []HttpDocApi
}

type HttpDocApi struct {
	Name        string
	Path        string
	Method      string
	ContentType string
	Request     []Struct
	Response    []Struct
}

type Struct struct {
	Name  string
	Field []Field
}

type Field struct {
	Name   string
	Type   string
	Remark string
}

func structInfo(input reflect.Type) []Struct {
	list := make([]reflect.Type, 0, 1)
	list = append(list, input)
	result := make([]Struct, 0, 1)
	for len(list) > 0 {
		process := append([]reflect.Type{}, list...)
		list = make([]reflect.Type, 0)

		for _, t := range process {
			if t.Kind() == reflect.Ptr {
				t = t.Elem() // use Elem to get the pointed-to-type
			}
			if t.Kind() == reflect.Slice {
				t = t.Elem() // use Elem to get type of slice's element
			}
			if t.Kind() == reflect.Ptr { // handle input of type like []*StructType
				t = t.Elem() // use Elem to get the pointed-to-type
			}
			if t.Kind() != reflect.Struct {
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

				if fieldInfo.Type.Kind() == reflect.Struct {
					list = append(list, fieldInfo.Type)
				} else if fieldInfo.Type.Kind() == reflect.Ptr {
					list = append(list, fieldInfo.Type.Elem())
				} else if fieldInfo.Type.Kind() == reflect.Slice {
					list = append(list, fieldInfo.Type.Elem())
				}

				name := fieldInfo.Name

				if jt := fieldInfo.Tag.Get("json"); jt != "" {
					name = strings.Split(jt, ",")[0]
				}

				f := Field{
					Name:   name,
					Type:   fieldInfo.Type.String(),
					Remark: fieldInfo.Tag.Get("remark"),
				}

				curr.Field = append(curr.Field, f)
			}

			result = append(result, curr)
		}

	}

	return result
}

const apiDocTmpl = `
# 接口文档

{{range $i,$d := .}}
## 模块：{{$d.Name}}
{{range $ii,$dd := $d.Api}}
### 接口：{{$dd.Name}}

路径：{{$dd.Path}}

请求方式：{{$dd.Method}}

{{range $iii,$ddd := $dd.Request}}
请求参数：{{$ddd.Name}}
| 数据项   | 名称         | 类型        | 必填 | 说明                           |
|-------|------------|-----------| -------- | -------------------------------------- |
{{range $fi,$dddd := $ddd.Field}}|{{$dddd.Remark}}|{{$dddd.Name}}|{{$dddd.Type}}| | |
{{end}}
{{end}}

{{range $iii,$ddd := $dd.Response}}
返回参数：{{$ddd.Name}}
| 数据项   | 名称         | 类型        | 必填 | 说明                           |
|-------|------------|-----------| -------- | -------------------------------------- |
{{range $fi,$dddd := $ddd.Field}}|{{$dddd.Remark}}|{{$dddd.Name}}|{{$dddd.Type}}| | |
{{end}}
{{end}}

{{end}}
{{end}}
`
