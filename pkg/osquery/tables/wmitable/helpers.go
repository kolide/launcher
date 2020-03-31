package wmitable

import "strings"

const allowedCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-"

func onlyAllowedCharacters(input string) bool {
	for _, char := range input {
		if !strings.ContainsRune(allowedCharacters, char) {
			return false
		}
	}
	return true
}
