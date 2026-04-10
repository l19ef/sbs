package builder

import "encoding/json"

type Outbound struct {
	Tag        string         `json:"tag"`
	Type       string         `json:"type"`
	Server     string         `json:"server,omitempty"`
	ServerPort int            `json:"server_port,omitempty"`
	TLS        map[string]any `json:"tls,omitempty"`
	Transport  map[string]any `json:"transport,omitempty"`

	UUID           string         `json:"uuid,omitempty"`
	Password       string         `json:"password,omitempty"`
	Method         string         `json:"method,omitempty"`
	Plugin         string         `json:"plugin,omitempty"`
	PluginOpts     string         `json:"plugin_opts,omitempty"`
	PacketEncoding string         `json:"packet_encoding,omitempty"`
	Flow           string         `json:"flow,omitempty"`
	Security       string         `json:"security,omitempty"`
	AlterID        *int           `json:"alter_id,omitempty"`
	UpMbps         *int           `json:"up_mbps,omitempty"`
	DownMbps       *int           `json:"down_mbps,omitempty"`
	Obfs           map[string]any `json:"obfs,omitempty"`
}

func cloneTypedOutboundList(items []Outbound) []Outbound {
	cloned := make([]Outbound, 0, len(items))
	for _, item := range items {
		clone := item
		if item.AlterID != nil {
			clone.AlterID = intPtr(*item.AlterID)
		}
		if item.UpMbps != nil {
			clone.UpMbps = intPtr(*item.UpMbps)
		}
		if item.DownMbps != nil {
			clone.DownMbps = intPtr(*item.DownMbps)
		}
		if item.TLS != nil {
			clone.TLS = cloneMap(item.TLS)
		}
		if item.Transport != nil {
			clone.Transport = cloneMap(item.Transport)
		}
		if item.Obfs != nil {
			clone.Obfs = cloneMap(item.Obfs)
		}
		cloned = append(cloned, clone)
	}
	return cloned
}

func collectOutboundTags(items []Outbound) []string {
	tags := make([]string, 0, len(items))
	for _, item := range items {
		if item.Tag != "" {
			tags = append(tags, item.Tag)
		}
	}
	return tags
}

func outboundFingerprint(outbound Outbound) (string, error) {
	payload, err := json.Marshal(outbound)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}
