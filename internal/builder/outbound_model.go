package builder

type Outbound struct {
	Tag        string
	Type       string
	Server     string
	ServerPort int
	TLS        map[string]any
	Transport  map[string]any
	Extra      map[string]any
}

func (o Outbound) ToMap() map[string]any {
	out := map[string]any{
		"tag":  o.Tag,
		"type": o.Type,
	}

	if o.Server != "" {
		out["server"] = o.Server
	}
	if o.ServerPort != 0 {
		out["server_port"] = o.ServerPort
	}
	if o.TLS != nil {
		out["tls"] = o.TLS
	}
	if o.Transport != nil {
		out["transport"] = o.Transport
	}

	for key, value := range o.Extra {
		if _, exists := out[key]; exists {
			continue
		}
		out[key] = value
	}

	return out
}
