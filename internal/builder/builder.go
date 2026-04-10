package builder

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func BuildFromFile(path string) ([]byte, error) {
	return BuildFromFileWithOptions(path, BuildOptions{})
}

func BuildFromFileWithOptions(path string, options BuildOptions) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read template: %w", err)
	}

	baseDir := filepath.Dir(path)
	return BuildWithOptions(data, baseDir, DefaultLoader(), options)
}

func Build(templateData []byte, baseDir string, loader SubscriptionContentLoader) ([]byte, error) {
	return BuildWithOptions(templateData, baseDir, loader, BuildOptions{})
}

func BuildWithOptions(templateData []byte, baseDir string, loader SubscriptionContentLoader, options BuildOptions) ([]byte, error) {
	raw := map[string]any{}
	if err := json.Unmarshal(templateData, &raw); err != nil {
		return nil, fmt.Errorf("parse template json: %w", err)
	}

	subscriptionByTag, err := extractRootSubscriptions(raw)
	if err != nil {
		return nil, err
	}

	rootOutbounds, err := ensureObjectArray(raw, "outbounds")
	if err != nil {
		return nil, err
	}

	resolver := &subscriptionResolver{
		byTag:  subscriptionByTag,
		cache:  map[string][]map[string]any{},
		loader: loader,
	}

	if err := expandSubscriptions(raw, resolver); err != nil {
		return nil, err
	}

	seenTopLevelTags := make(map[string]struct{}, len(rootOutbounds))
	for _, outbound := range rootOutbounds {
		tag, _ := outbound["tag"].(string)
		if tag != "" {
			seenTopLevelTags[tag] = struct{}{}
		}
	}

	for _, items := range resolver.cache {
		for _, outbound := range items {
			tag, _ := outbound["tag"].(string)
			if tag == "" {
				return nil, fmt.Errorf("resolved outbound without tag")
			}
			if _, exists := seenTopLevelTags[tag]; exists {
				continue
			}
			rootOutbounds = append(rootOutbounds, outbound)
			seenTopLevelTags[tag] = struct{}{}
		}
	}
	raw["outbounds"] = rootOutbounds

	result, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode result json: %w", err)
	}
	return append(result, '\n'), nil
}

func extractRootSubscriptions(root map[string]any) (map[string]subscriptionSource, error) {
	rawSubscriptions, exists := root["subscriptions"]
	if !exists {
		return map[string]subscriptionSource{}, nil
	}
	delete(root, "subscriptions")

	data, err := json.Marshal(rawSubscriptions)
	if err != nil {
		return nil, fmt.Errorf("marshal subscriptions: %w", err)
	}

	var items []subscriptionSource
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("unmarshal subscriptions: %w", err)
	}

	subscriptionByTag := make(map[string]subscriptionSource, len(items))
	for _, item := range items {
		if item.Tag == "" {
			return nil, fmt.Errorf("subscription tag cannot be empty")
		}
		if item.URL == "" {
			return nil, fmt.Errorf("subscription %q must define url", item.Tag)
		}
		if _, exists := subscriptionByTag[item.Tag]; exists {
			return nil, fmt.Errorf("duplicate subscription tag %q", item.Tag)
		}
		subscriptionByTag[item.Tag] = item
	}

	return subscriptionByTag, nil
}
