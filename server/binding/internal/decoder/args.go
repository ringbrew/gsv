package decoder

import "net/url"

func peekArgStr(v url.Values, k string) string {
	for ke, vv := range v {
		if ke == k && len(vv) > 0 {
			return vv[0]
		}
	}
	return ""
}

func peekArgStrExists(v url.Values, k string) (string, bool) {
	for ke, vv := range v {
		if ke == k && len(vv) > 0 {
			return vv[0], true
		}
	}
	return "", false
}
