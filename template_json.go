package zas

import (
	"encoding/json"
	"fmt"
	"html/template"
)

// templateDict constructs JSON objects without silently overwriting keys.
func templateDict(pairs ...interface{}) (map[string]interface{}, error) {
	if len(pairs)%2 != 0 {
		return nil, fmt.Errorf("dict requires key/value pairs, got %d arguments", len(pairs))
	}
	object := make(map[string]interface{}, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict key %d must be a string, got %T", i/2+1, pairs[i])
		}
		if _, exists := object[key]; exists {
			return nil, fmt.Errorf("dict has duplicate key %q", key)
		}
		object[key] = pairs[i+1]
	}
	return object, nil
}

// templateList preserves an empty list as [] rather than JSON null.
func templateList(values ...interface{}) []interface{} {
	items := make([]interface{}, len(values))
	copy(items, values)
	return items
}

// templateJSON marks only the serializer's own output as a JSON expression.
// Marshal's default HTML escaping prevents closing a script element, including
// when the input implements json.Marshaler or contains json.RawMessage.
func templateJSON(value interface{}) (template.JS, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("json: %w", err)
	}
	return template.JS(encoded), nil
}
