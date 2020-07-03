package wmitable

import "strings"

const allowedCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-"

func onlyAllowedCharacters(input string, extras ...string) bool {

	allowed := strings.Join(append(extras, allowedCharacters), "")
	for _, char := range input {
		if !strings.ContainsRune(allowed, char) {
			return false
		}
	}
	return true
}
