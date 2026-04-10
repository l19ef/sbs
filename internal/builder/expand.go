package builder

import (
	"fmt"
	"slices"
)

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

		for _, subscriptionTag := range subscriptionTags {
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
