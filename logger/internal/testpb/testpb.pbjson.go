package testpb

import "google.golang.org/protobuf/encoding/protojson"

// Same as the output of protoc-gen-go-gsv.

func (x *Inner) MarshalJSON() ([]byte, error) {
	return protojson.MarshalOptions{UseEnumNumbers: true, EmitUnpopulated: true}.Marshal(x)
}

func (x *Account) MarshalJSON() ([]byte, error) {
	return protojson.MarshalOptions{UseEnumNumbers: true, EmitUnpopulated: true}.Marshal(x)
}

func (x *QueryResp) MarshalJSON() ([]byte, error) {
	return protojson.MarshalOptions{UseEnumNumbers: true, EmitUnpopulated: true}.Marshal(x)
}

func (x *Clean) MarshalJSON() ([]byte, error) {
	return protojson.MarshalOptions{UseEnumNumbers: true, EmitUnpopulated: true}.Marshal(x)
}
