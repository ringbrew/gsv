package discovery

import "fmt"

func Scheme(service string) string {
	return fmt.Sprintf("%s:///%s", SchemeName, service)
}
