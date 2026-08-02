package inventory

import "fmt"

func Report(s *Store, skus []string) string {
	out := ""
	for _, k := range skus {
		out += fmt.Sprintf("%s=%d\n", k, s.Get(k))
	}
	return out
}
