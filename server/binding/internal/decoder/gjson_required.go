//go:build gjson || !(amd64 && (linux || windows || darwin))
// +build gjson !amd64 !linux,!windows,!darwin

package decoder

import (
	"github.com/ringbrew/gsv/server/binding/common"
	"github.com/ringbrew/gsv/server/binding/consts"
	"github.com/tidwall/gjson"
	"strings"
)

func checkRequireJSON(decodeInput *DecodeInput, tagInfo TagInfo) bool {
	if !tagInfo.Required {
		return true
	}
	if !strings.EqualFold(common.FilterContentType(decodeInput.ContentType()), consts.MIMEApplicationJSON) {
		return false
	}

	result := gjson.GetBytes(decodeInput.Body(), tagInfo.JSONName)
	if !result.Exists() {
		idx := strings.LastIndex(tagInfo.JSONName, ".")
		// There should be a superior if it is empty, it will report 'true' for required
		if idx > 0 && !gjson.GetBytes(decodeInput.Body(), tagInfo.JSONName[:idx]).Exists() {
			return true
		}
		return false
	}
	return true
}

func keyExist(decodeInput *DecodeInput, tagInfo TagInfo) bool {
	if common.FilterContentType(decodeInput.ContentType()) != consts.MIMEApplicationJSON {
		return false
	}
	result := gjson.GetBytes(decodeInput.Body(), tagInfo.JSONName)
	return result.Exists()
}
