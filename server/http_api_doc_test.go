package server

import (
	"log"
	"reflect"
	"testing"
)

type A struct {
	B
}

type B struct {
	C C
}
type C struct {
	D string
	E int
}

func TestDoc(t *testing.T) {
	data := A{}
	result := structInfo(reflect.TypeOf(data))
	for _, v := range result {
		log.Println(v)
	}
}
