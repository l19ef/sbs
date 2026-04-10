package builder

import (
	"encoding/json"
	"fmt"
	"strings"
)

func parseVMessLine(line, defaultTag string) (Outbound, error) {
	payload := strings.TrimPrefix(line, "vmess://")
	if payload == "" {
		return Outbound{}, fmt.Errorf("empty vmess url")
	}

	decoded, err := decodeBase64String(payload)
	if err != nil {
		return Outbound{}, fmt.Errorf("decode vmess base64: %w", err)
	}

	decodedStr := strings.TrimSpace(string(decoded))
	if !strings.HasPrefix(decodedStr, "{") {
		return Outbound{}, fmt.Errorf("invalid vmess format: expected JSON")
	}

	return parseVMessJSON(decodedStr, defaultTag)
}

func parseVMessJSON(payload, defaultTag string) (Outbound, error) {
	var item map[string]any
	if err := json.Unmarshal([]byte(payload), &item); err != nil {
		return Outbound{}, fmt.Errorf("decode vmess json: %w", err)
	}

	port, err := parseIntString(anyToString(item["port"]))
	if err != nil {
		return Outbound{}, fmt.Errorf("invalid vmess port: %w", err)
	}

	outbound := Outbound{
		Tag:            firstNonEmpty(strings.TrimSpace(anyToString(item["ps"])), defaultTag),
		Type:           "vmess",
		Server:         anyToString(item["add"]),
		ServerPort:     port,
		UUID:           anyToString(item["id"]),
		Security:       normalizeVMessSecurity(anyToString(item["scy"])),
		AlterID:        intPtr(parseOptionalInt(anyToString(item["aid"]))),
		PacketEncoding: "xudp",
	}

	netType := anyToString(item["net"])
	if tlsValue := anyToString(item["tls"]); tlsValue != "" && tlsValue != "none" {
		tls := map[string]any{
			"enabled":     true,
			"insecure":    true,
			"server_name": "",
		}
		if netType != "h2" && netType != "http" {
			tls["server_name"] = anyToString(item["host"])
		}
		if sni := anyToString(item["sni"]); sni != "" {
			tls["server_name"] = sni
		}
		outbound.TLS = tls
	}

	outbound.Transport = buildVMessTransport(netType, item)
	return outbound, nil
}

func normalizeVMessSecurity(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case "", "http", "gun":
		return "auto"
	default:
		return value
	}
}

func buildVMessTransport(netType string, item map[string]any) map[string]any {
	switch netType {
	case "h2", "http", "tcp":
		transport := map[string]any{"type": "http"}
		if host := anyToString(item["host"]); host != "" {
			transport["host"] = host
		}
		if path := anyToString(item["path"]); path != "" {
			transport["path"] = strings.Split(path, "?")[0]
		}
		return transport
	case "ws":
		transport := map[string]any{"type": "ws"}
		if host := anyToString(item["host"]); host != "" {
			transport["headers"] = map[string]any{"Host": host}
		}
		if path := anyToString(item["path"]); path != "" {
			basePath, earlyData := splitEarlyDataPath(path)
			transport["path"] = basePath
			if earlyData > 0 {
				transport["early_data_header_name"] = "Sec-WebSocket-Protocol"
				transport["max_early_data"] = earlyData
			}
		}
		return transport
	case "grpc":
		return map[string]any{
			"type":         "grpc",
			"service_name": anyToString(item["path"]),
		}
	case "quic":
		return map[string]any{"type": "quic"}
	}

	return nil
}
