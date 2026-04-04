package builder

type subscriptionSource struct {
	Tag string `json:"tag"`
	URL string `json:"url"`
}

type outboundContainer struct {
	Outbounds []map[string]any `json:"outbounds"`
}

type BuildOptions struct {
	Emojify          bool
	ExcludePatterns  []string
	ExcludeProtocols []string
}
