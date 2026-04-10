package builder

import (
	"fmt"
	"net/url"
	"strings"
)

func parseTrojanLine(line, defaultTag string) (Outbound, error) {
	u, err := url.Parse(line)
	if err != nil {
		return Outbound{}, fmt.Errorf("parse trojan url: %w", err)
	}
	if u.User == nil {
		return Outbound{}, fmt.Errorf("trojan url is missing password")
	}

	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return Outbound{}, err
	}

	outbound := Outbound{
		Tag:        parseTag(u.Fragment, defaultTag),
		Type:       "trojan",
		Server:     host,
		ServerPort: port,
		Password:   u.User.Username(),
		TLS: map[string]any{
			"enabled":  true,
			"insecure": false,
		},
	}

	query := u.Query()
	tls := outbound.TLS
	if sni := firstNonEmpty(query.Get("sni"), query.Get("peer")); sni != "" {
		tls["server_name"] = sni
	}
	if query.Get("allowInsecure") == "1" || strings.EqualFold(query.Get("allowInsecure"), "true") {
		tls["insecure"] = true
	}
	if alpn := parseCSVList(query.Get("alpn")); len(alpn) > 0 {
		tls["alpn"] = alpn
	}
	if fp := query.Get("fp"); fp != "" {
		tls["utls"] = map[string]any{
			"enabled":     true,
			"fingerprint": fp,
		}
	}

	outbound.Transport = buildTransport(query)
	return outbound, nil
}

func parseVLESSLine(line, defaultTag string) (Outbound, error) {
	u, err := url.Parse(line)
	if err != nil {
		return Outbound{}, fmt.Errorf("parse vless url: %w", err)
	}
	if u.User == nil {
		return Outbound{}, fmt.Errorf("vless url is missing uuid")
	}

	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return Outbound{}, err
	}

	query := u.Query()
	outbound := Outbound{
		Tag:            parseTag(u.Fragment, defaultTag),
		Type:           "vless",
		Server:         host,
		ServerPort:     port,
		UUID:           u.User.Username(),
		PacketEncoding: firstNonEmpty(query.Get("packetEncoding"), "xudp"),
	}

	if flow := query.Get("flow"); flow != "" {
		outbound.Flow = flow
	}

	if shouldEnableTLS(query) {
		tls := map[string]any{
			"enabled":  true,
			"insecure": false,
		}
		if sni := firstNonEmpty(query.Get("sni"), query.Get("peer")); sni != "" && !strings.EqualFold(sni, "none") {
			tls["server_name"] = sni
		}
		if query.Get("allowInsecure") == "1" || strings.EqualFold(query.Get("allowInsecure"), "true") {
			tls["insecure"] = true
		}
		if security := query.Get("security"); security == "reality" || query.Get("pbk") != "" {
			reality := map[string]any{
				"enabled":    true,
				"public_key": query.Get("pbk"),
			}
			if sid := query.Get("sid"); sid != "" && !strings.EqualFold(sid, "none") {
				reality["short_id"] = sid
			}
			tls["reality"] = reality
			utls := map[string]any{"enabled": true}
			if fp := query.Get("fp"); fp != "" {
				utls["fingerprint"] = fp
			}
			tls["utls"] = utls
		}
		outbound.TLS = tls
	}

	outbound.Transport = buildTransport(query)
	return outbound, nil
}

func buildTransport(query url.Values) map[string]any {
	switch query.Get("type") {
	case "ws":
		path, earlyData := splitEarlyDataPath(firstNonEmpty(query.Get("path"), "/"))
		transport := map[string]any{
			"type": "ws",
			"path": path,
		}
		if host := query.Get("host"); host != "" {
			transport["headers"] = map[string]any{"Host": host}
		}
		if earlyData > 0 {
			transport["early_data_header_name"] = "Sec-WebSocket-Protocol"
			transport["max_early_data"] = earlyData
		}
		return transport
	case "grpc":
		return map[string]any{
			"type":         "grpc",
			"service_name": query.Get("serviceName"),
		}
	case "h2", "http":
		transport := map[string]any{
			"type": "http",
			"path": firstNonEmpty(query.Get("path"), "/"),
		}
		if host := firstNonEmpty(query.Get("host"), query.Get("sni")); host != "" {
			transport["host"] = host
		}
		return transport
	}

	return nil
}
