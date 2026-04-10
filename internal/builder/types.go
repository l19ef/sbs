package builder

type subscriptionSource struct {
	Tag              string   `json:"tag"`
	URL              string   `json:"url"`
	Emojify          bool     `json:"emojify"`
	Exclude          []string `json:"exclude"`
	ExcludeProtocols []string `json:"exclude_protocols"`
	Encoding         string   `json:"encoding"`
}

type outboundContainer struct {
	Outbounds []map[string]any `json:"outbounds"`
}

type BuildOptions struct {
	Emojify          bool
	ExcludePatterns  []string
	ExcludeProtocols []string
	Encoding         string
}
