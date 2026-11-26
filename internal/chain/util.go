package chain

import "fmt"

func sprintf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}

func hasBasis(basis []string, want string) bool {
	for _, b := range basis {
		if b == want {
			return true
		}
	}
	return false
}
