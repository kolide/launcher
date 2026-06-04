package katc

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/kolide/launcher/v2/ee/dataflatten"
	"github.com/kolide/launcher/v2/ee/tables/dataflattentable"
)

// maxRecursiveUnmarshalDepth caps recursivelyUnmarshal so a pathological input
// cannot drive unbounded recursion.
const maxRecursiveUnmarshalDepth = 100

// flattenRow turns one post-transform row into dataflatten-style output rows.
// Each value is recursively JSON-unmarshalled first so values produced by
// IndexedDB deserialization (which encode complex types as JSON strings,
// sometimes nested) are expanded into their structured form before flattening.
// dataQuery is a dataflatten query string (split on "/"); pass "*" to match all.
func flattenRow(slogger *slog.Logger, row map[string][]byte, path string, dataQuery string) ([]map[string]string, error) {
	flatInput := make(map[string]any, len(row))
	for k, v := range row {
		flatInput[k] = recursivelyUnmarshal(string(v), 0, maxRecursiveUnmarshalDepth)
	}

	flatRows, err := dataflatten.Flatten(flatInput,
		dataflatten.WithSlogger(slogger),
		dataflatten.WithQuery(strings.Split(dataQuery, "/")),
	)
	if err != nil {
		return nil, fmt.Errorf("flattening row: %w", err)
	}

	return dataflattentable.ToMap(flatRows, dataQuery, map[string]string{pathColumnName: path}), nil
}

// recursivelyUnmarshal walks the given value, attempting to JSON-unmarshal any
// string it encounters. Strings that aren't valid JSON pass through unchanged.
func recursivelyUnmarshal(potentialJson any, currentDepth int, maxDepth int) any {
	if currentDepth > maxDepth {
		return potentialJson
	}
	if potentialJsonMap, ok := potentialJson.(map[string]any); ok {
		for k, v := range potentialJsonMap {
			potentialJsonMap[k] = recursivelyUnmarshal(v, currentDepth+1, maxDepth)
		}
		return potentialJsonMap
	}
	if potentialJsonArr, ok := potentialJson.([]any); ok {
		for i, v := range potentialJsonArr {
			potentialJsonArr[i] = recursivelyUnmarshal(v, currentDepth+1, maxDepth)
		}
		return potentialJsonArr
	}
	if potentialJsonStr, ok := potentialJson.(string); ok {
		var j any
		if err := json.Unmarshal([]byte(potentialJsonStr), &j); err != nil {
			return potentialJson
		}
		return recursivelyUnmarshal(j, currentDepth+1, maxDepth)
	}
	return potentialJson
}
