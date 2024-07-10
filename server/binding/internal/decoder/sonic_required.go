//go:build (linux || windows || darwin) && amd64 && !gjson
// +build linux windows darwin
// +build amd64
// +build !gjson

package decoder

import (
	"github.com/bytedance/sonic"
	"github.com/ringbrew/gsv/server/binding/consts"
	"strings"
)

func checkRequireJSON(decodeInput DecodeInput, tagInfo TagInfo) bool {
	if !tagInfo.Required {
		return true
	}
	if !strings.EqualFold(decodeInput.ContentType(), consts.MIMEApplicationJSON) {
		return false
	}
	node, _ := sonic.Get(decodeInput.Body(), stringSliceForInterface(tagInfo.JSONName)...)
	if !node.Exists() {
		idx := strings.LastIndex(tagInfo.JSONName, ".")
		if idx > 0 {
			// There should be a superior if it is empty, it will report 'true' for required
			node, _ := sonic.Get(decodeInput.Body(), stringSliceForInterface(tagInfo.JSONName[:idx])...)
			if !node.Exists() {
				return true
			}
		}
		return false
	}
	return true
}

func stringSliceForInterface(s string) (ret []interface{}) {
	x := strings.Split(s, ".")
	for _, val := range x {
		ret = append(ret, val)
	}
	return
}

func keyExist(decodeInput DecodeInput, tagInfo TagInfo) bool {
	if utils.FilterContentType(decodeInput.ContentType()) != consts.MIMEApplicationJSON {
		return false
	}
	node, _ := sonic.Get(decodeInput.Body(), stringSliceForInterface(tagInfo.JSONName)...)
	return node.Exists()
}
