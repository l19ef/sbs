package builder

import (
	"context"
	"fmt"
)

type subscriptionResolver struct {
	byTag  map[string]subscriptionSource
	cache  map[string][]map[string]any
	loader SubscriptionContentLoader
}

func (r *subscriptionResolver) resolve(tag string) ([]map[string]any, error) {
	if items, ok := r.cache[tag]; ok {
		return cloneOutboundList(items), nil
	}

	source, ok := r.byTag[tag]
	if !ok {
		return nil, fmt.Errorf("subscription %q is not defined", tag)
	}

	if r.loader == nil {
		return nil, fmt.Errorf("no subscription loader configured")
	}

	data, err := r.loader.Load(context.Background(), source)
	if err != nil {
		return nil, fmt.Errorf("load subscription %q: %w", tag, err)
	}

	options := BuildOptions{
		Emojify:          source.Emojify,
		ExcludePatterns:  source.Exclude,
		ExcludeProtocols: source.ExcludeProtocols,
		Encoding:         source.Encoding,
	}

	items, err := parseSubscriptionContent(data, tag, options)
	if err != nil {
		return nil, fmt.Errorf("parse subscription %q: %w", tag, err)
	}

	for _, outbound := range items {
		if _, ok := outbound["tag"].(string); !ok {
			return nil, fmt.Errorf("subscription %q contains outbound without string tag", tag)
		}
	}

	r.cache[tag] = cloneOutboundList(items)
	return cloneOutboundList(items), nil
}
