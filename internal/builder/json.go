package builder

import (
	"encoding/json"
	"fmt"
)

func parseOutbounds(data []byte) ([]map[string]any, error) {
	var list []map[string]any
	if err := json.Unmarshal(data, &list); err == nil {
		return list, nil
	}

	var wrapped outboundContainer
	if err := json.Unmarshal(data, &wrapped); err == nil && wrapped.Outbounds != nil {
		return wrapped.Outbounds, nil
	}

	return nil, fmt.Errorf("expected JSON array or object with outbounds field")
}

func ensureObjectArray(root map[string]any, key string) ([]map[string]any, error) {
	raw, exists := root[key]
	if !exists {
		items := []map[string]any{}
		root[key] = items
		return items, nil
	}

	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%q must be an array", key)
	}

	items := make([]map[string]any, 0, len(list))
	for _, entry := range list {
		item, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%q must contain only objects", key)
		}
		items = append(items, item)
	}
	return items, nil
}

func ensureStringArrayIfPresent(root map[string]any, key string) ([]string, error) {
	raw, exists := root[key]
	if !exists {
		return nil, nil
	}

	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%q must be an array of strings", key)
	}

	items := make([]string, 0, len(list))
	for _, entry := range list {
		value, ok := entry.(string)
		if !ok {
			return nil, fmt.Errorf("%q must be an array of strings", key)
		}
		items = append(items, value)
	}
	return items, nil
}

func collectTags(items []map[string]any) []string {
	tags := make([]string, 0, len(items))
	for _, item := range items {
		if tag, ok := item["tag"].(string); ok && tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

func cloneOutboundList(items []map[string]any) []map[string]any {
	cloned := make([]map[string]any, 0, len(items))
	for _, item := range items {
		cloned = append(cloned, cloneMap(item))
	}
	return cloned
}

func cloneMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = cloneValue(value)
	}
	return out
}

func cloneSlice(input []any) []any {
	out := make([]any, len(input))
	for i, value := range input {
		out[i] = cloneValue(value)
	}
	return out
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		return cloneSlice(typed)
	default:
		return typed
	}
}
