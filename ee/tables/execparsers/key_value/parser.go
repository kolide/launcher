package key_value

import (
	"fmt"
	"io"
	"strings"
)

// parseKeyValue implements the parsing logic for key-value data.
// It parses delimiter-separated key-value pairs from the input
// and handles duplicate keys by creating arrays (similar to dataflatten.DuplicateKeys)
func (p *parser) parseKeyValue(reader io.Reader) (any, error) {
	// Read all data from the reader
	rawdata, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("reading from reader: %w", err)
	}

	content := string(rawdata)

	// Strip UTF-8 BOM if present.
	// The UTF-8 BOM is the Unicode character U+FEFF.
	// When encoded in UTF-8, it becomes the byte sequence EF BB BF.
	// In a Go string, "\uFEFF" represents this character.
	content = strings.TrimPrefix(content, "\uFEFF")

	// Normalize line endings: replace \r\n with \n, then \r with \n
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	// Split into lines
	lines := strings.Split(content, "\n")

	result := make(map[string]interface{})

	for _, line := range lines {
		// Skip empty lines and comments
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		// Split on the delimiter (e.g., "=" or ":")
		parts := strings.SplitN(line, p.kvDelimiter, 2)
		if len(parts) != 2 {
			continue // Skip malformed lines
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		if len(value) >= 2 {
			if (strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`)) ||
				(strings.HasPrefix(value, `'`) && strings.HasSuffix(value, `'`)) {
				value = value[1 : len(value)-1]
			}
		}

		// Handle nested keys (e.g., "section.subsection.key")
		if strings.Contains(key, ".") {
			p.setNestedValueWithDuplicates(result, key, value)
		} else {
			p.setValueWithDuplicates(result, key, value)
		}
	}

	return result, nil
}

// setValueWithDuplicates handles duplicate keys by creating arrays
// This mimics the behavior of dataflatten.DuplicateKeys
func (p *parser) setValueWithDuplicates(result map[string]interface{}, key string, value string) {
	if existing, exists := result[key]; exists {
		// Key already exists, handle duplicates
		switch v := existing.(type) {
		case []interface{}:
			// Already an array, append to it
			result[key] = append(v, value)
		default:
			// Convert to array with existing value and new value
			result[key] = []interface{}{v, value}
		}
	} else {
		// First occurrence of this key
		result[key] = value
	}
}

// setNestedValueWithDuplicates sets a value in a nested map structure using dot notation
// and handles duplicate keys by creating arrays
func (p *parser) setNestedValueWithDuplicates(result map[string]interface{}, key string, value string) {
	parts := strings.Split(key, ".")
	current := result

	// Navigate/create the nested structure
	for _, part := range parts[:len(parts)-1] {
		if _, exists := current[part]; !exists {
			current[part] = make(map[string]interface{})
		}

		if nested, ok := current[part].(map[string]interface{}); ok {
			current = nested
		} else {
			// Handle conflict - convert to map if a non-map value exists at this path
			// This ensures that if 'parent = "string"' was set and then 'parent.child = "value"' comes,
			// 'parent' becomes a map.
			current[part] = make(map[string]interface{})
			current = current[part].(map[string]interface{})
		}
	}

	// Set the final value with duplicate handling
	finalKey := parts[len(parts)-1]
	p.setValueWithDuplicates(current, finalKey, value)
}
