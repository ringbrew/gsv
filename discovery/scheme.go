package discovery

import "fmt"

func Scheme(service string, localOpt ...map[string]string) string {
	if len(localOpt) > 0 {
		if val, exist := localOpt[0][service]; exist {
			return val
		}
	}

	return fmt.Sprintf("%s:///%s", SchemeName, service)
}
