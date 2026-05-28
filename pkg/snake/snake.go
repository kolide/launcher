// Package snake provides minimal helpers for converting strings between
// camelCase/PascalCase and snake_case. It is a small in-tree replacement for
// the previously-vendored github.com/serenize/snaker package, and preserves
// that package's handling of common Go-style initialisms (e.g. ID, URL, HTTP)
// so existing call sites continue to produce the same output.
package snake

import (
	"strings"
	"unicode"
)

// commonInitialisms mirrors the list used by github.com/golang/lint's golint
// (and previously by serenize/snaker), so converted output keeps initialisms
// such as "ID" or "URL" as a single word rather than splitting them up.
var commonInitialisms = map[string]bool{
	"ACL":   true,
	"API":   true,
	"ASCII": true,
	"CPU":   true,
	"CSS":   true,
	"DNS":   true,
	"EOF":   true,
	"ETA":   true,
	"GPU":   true,
	"GUID":  true,
	"HTML":  true,
	"HTTP":  true,
	"HTTPS": true,
	"ID":    true,
	"IP":    true,
	"JSON":  true,
	"LHS":   true,
	"OS":    true,
	"QPS":   true,
	"RAM":   true,
	"RHS":   true,
	"RPC":   true,
	"SLA":   true,
	"SMTP":  true,
	"SQL":   true,
	"SSH":   true,
	"TCP":   true,
	"TLS":   true,
	"TTL":   true,
	"UDP":   true,
	"UI":    true,
	"UID":   true,
	"UUID":  true,
	"URI":   true,
	"URL":   true,
	"UTF8":  true,
	"VM":    true,
	"XML":   true,
	"XMPP":  true,
	"XSRF":  true,
	"XSS":   true,
	"OAuth": true,
}

// snakeToCamelExceptions maps lowercase snake words to their preferred camel
// rendering when the default (just upper-casing the first letter) is wrong.
var snakeToCamelExceptions = map[string]string{
	"oauth": "OAuth",
}

// CamelToSnake converts a camelCase or PascalCase string to snake_case.
// Recognized initialisms are kept together as a single word, e.g.
// "userID" -> "user_id" rather than "user_i_d".
func CamelToSnake(s string) string {
	var words []string
	lastPos := 0
	rs := []rune(s)

	for i := 0; i < len(rs); i++ {
		if i == 0 || !unicode.IsUpper(rs[i]) {
			continue
		}

		if initialism := startsWithInitialism(s[lastPos:]); initialism != "" {
			words = append(words, initialism)
			i += len(initialism) - 1
			lastPos = i
			continue
		}

		words = append(words, s[lastPos:i])
		lastPos = i
	}

	if s[lastPos:] != "" {
		words = append(words, s[lastPos:])
	}

	var b strings.Builder
	for k, word := range words {
		if k > 0 {
			b.WriteByte('_')
		}
		b.WriteString(strings.ToLower(word))
	}
	return b.String()
}

// SnakeToCamel converts a snake_case string to PascalCase, capitalizing the
// first letter of each underscore-separated segment. Segments matching a
// recognized initialism are emitted in upper case (e.g. "user_id" -> "UserID").
func SnakeToCamel(s string) string {
	return convertSnakeToCamel(s, true)
}

// SnakeToCamelLower converts a snake_case string to camelCase, leaving the
// first segment lowercase.
func SnakeToCamelLower(s string) string {
	return convertSnakeToCamel(s, false)
}

func convertSnakeToCamel(s string, upperFirst bool) string {
	var b strings.Builder
	for i, word := range strings.Split(s, "_") {
		if exception, ok := snakeToCamelExceptions[word]; ok {
			b.WriteString(exception)
			continue
		}

		shouldCapitalize := upperFirst || i > 0
		if shouldCapitalize {
			if upper := strings.ToUpper(word); commonInitialisms[upper] {
				b.WriteString(upper)
				continue
			}
		}

		if shouldCapitalize && len(word) > 0 {
			rs := []rune(word)
			rs[0] = unicode.ToUpper(rs[0])
			b.WriteString(string(rs))
			continue
		}

		b.WriteString(word)
	}
	return b.String()
}

// startsWithInitialism returns the longest commonInitialism prefix of s, or
// the empty string if none match. Initialisms are at most 5 runes long.
func startsWithInitialism(s string) string {
	var initialism string
	for i := 1; i <= 5 && i <= len(s); i++ {
		if commonInitialisms[s[:i]] {
			initialism = s[:i]
		}
	}
	return initialism
}
