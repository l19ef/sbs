package builder

import (
	"fmt"
	"slices"
	"strings"
)

var linkProtocols = []string{
	"ss://",
	"trojan://",
	"vless://",
	"vmess://",
	"hysteria2://",
	"hy2://",
}

func isLink(s string) bool {
	s = strings.ToLower(s)
	for _, p := range linkProtocols {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func parseLink(link string, defaultTag string) (map[string]any, error) {
	outbound, recognized, err := parseSubscriptionLine(link, defaultTag, 0)
	if err != nil {
		return nil, err
	}
	if !recognized {
		return nil, fmt.Errorf("unrecognized link protocol: %s", link)
	}
	return outboundToMap(outbound), nil
}

func expandLink(node map[string]any) (map[string]any, error) {
	link, ok := node["link"].(string)
	if !ok || link == "" {
		return nil, fmt.Errorf("link field must be a non-empty string")
	}

	outbound, err := parseLink(link, "link")
	if err != nil {
		return nil, fmt.Errorf("parse link: %w", err)
	}

	return outbound, nil
}

func expandLinks(outbounds []map[string]any) error {
	result := make([]map[string]any, 0, len(outbounds))

	for _, outbound := range outbounds {
		if link, hasLink := outbound["link"]; hasLink {
			linkStr, isString := link.(string)
			if !isString || linkStr == "" {
				return fmt.Errorf("link field must be a non-empty string")
			}

			expanded, err := expandLink(outbound)
			if err != nil {
				return err
			}
			result = append(result, expanded)
		} else {
			result = append(result, outbound)
		}
	}

	for i := range outbounds {
		outbounds[i] = result[i]
	}

	return nil
}

func expandSubscriptions(node any, resolver *subscriptionResolver) error {
	switch typed := node.(type) {
	case map[string]any:
		for _, value := range typed {
			if err := expandSubscriptions(value, resolver); err != nil {
				return err
			}
		}

		subscriptionTags, hasSubscriptions, err := getObjectSubscriptions(typed)
		if err != nil {
			return err
		}
		if !hasSubscriptions {
			return nil
		}

		existing, err := ensureStringArrayIfPresent(typed, "outbounds")
		if err != nil {
			return err
		}

		expandedTags := make([]string, 0, len(subscriptionTags))
		seen := make(map[string]bool, len(subscriptionTags))
		for _, tag := range subscriptionTags {
			if tag == "*" {
				for _, kt := range resolver.knownTags() {
					if !seen[kt] {
						expandedTags = append(expandedTags, kt)
						seen[kt] = true
					}
				}
			} else if !seen[tag] {
				expandedTags = append(expandedTags, tag)
				seen[tag] = true
			}
		}

		for _, subscriptionTag := range expandedTags {
			items, err := resolver.resolve(subscriptionTag)
			if err != nil {
				return err
			}

			for _, tag := range collectOutboundTags(items) {
				if !slices.Contains(existing, tag) {
					existing = append(existing, tag)
				}
			}
		}
		typed["outbounds"] = existing
		delete(typed, "subscriptions")
	case []any:
		for _, item := range typed {
			if err := expandSubscriptions(item, resolver); err != nil {
				return err
			}
		}
	}
	return nil
}

func getObjectSubscriptions(node map[string]any) ([]string, bool, error) {
	raw, exists := node["subscriptions"]
	if !exists {
		return nil, false, nil
	}

	list, ok := raw.([]any)
	if !ok {
		return nil, true, fmt.Errorf("%q must be an array of strings when used inside an outbound", "subscriptions")
	}

	items := make([]string, 0, len(list))
	for _, entry := range list {
		value, ok := entry.(string)
		if !ok {
			return nil, true, fmt.Errorf("%q must be an array of strings when used inside an outbound", "subscriptions")
		}
		if value != "" && !slices.Contains(items, value) {
			items = append(items, value)
		}
	}

	return items, true, nil
}
