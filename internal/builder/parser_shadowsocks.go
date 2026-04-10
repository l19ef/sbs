package builder

import (
	"fmt"
	"net/url"
	"strings"
)

func parseShadowsocksLine(line, defaultTag string) (Outbound, error) {
	rest := strings.TrimPrefix(line, "ss://")
	mainPart, fragment := splitFragment(rest)
	mainPart, rawQuery := splitQuery(mainPart)

	outbound := Outbound{
		Tag:  parseTag(fragment, defaultTag),
		Type: "shadowsocks",
	}

	credentialsPart, serverPart, err := splitShadowsocksParts(mainPart)
	if err != nil {
		return Outbound{}, err
	}

	method, password, err := parseShadowsocksCredentials(credentialsPart)
	if err != nil {
		return Outbound{}, err
	}

	host, port, err := splitHostPort(serverPart)
	if err != nil {
		return Outbound{}, err
	}

	outbound.Server = host
	outbound.ServerPort = port
	outbound.Method = normalizeShadowsocksMethod(method)
	outbound.Password = password

	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return Outbound{}, fmt.Errorf("parse ss query: %w", err)
	}
	plugin, pluginOpts := parseShadowsocksPlugin(query.Get("plugin"))
	outbound.Plugin = plugin
	outbound.PluginOpts = pluginOpts

	return outbound, nil
}

func splitShadowsocksParts(mainPart string) (string, string, error) {
	at := strings.LastIndex(mainPart, "@")
	if at >= 0 {
		credentialsPart := mainPart[:at]
		serverPart := mainPart[at+1:]
		if credentialsPart == "" || serverPart == "" {
			return "", "", fmt.Errorf("invalid ss url")
		}
		return credentialsPart, serverPart, nil
	}

	decoded, err := decodeBase64String(mainPart)
	if err != nil {
		return "", "", fmt.Errorf("invalid ss url: missing @ separator")
	}
	decodedMain := strings.TrimSpace(string(decoded))
	decodedAt := strings.LastIndex(decodedMain, "@")
	if decodedAt < 0 {
		return "", "", fmt.Errorf("invalid ss url: missing @ separator")
	}

	credentialsPart := decodedMain[:decodedAt]
	serverPart := decodedMain[decodedAt+1:]
	if credentialsPart == "" || serverPart == "" {
		return "", "", fmt.Errorf("invalid ss url")
	}
	return credentialsPart, serverPart, nil
}

func parseShadowsocksCredentials(credentialsPart string) (string, string, error) {
	if decoded, err := decodeBase64String(credentialsPart); err == nil {
		if method, password, ok := strings.Cut(string(decoded), ":"); ok {
			return method, password, nil
		}
	}

	decodedUserInfo, err := url.QueryUnescape(credentialsPart)
	if err != nil {
		return "", "", fmt.Errorf("decode ss credentials: %w", err)
	}

	method, password, ok := strings.Cut(decodedUserInfo, ":")
	if !ok || method == "" {
		return "", "", fmt.Errorf("invalid ss credentials")
	}
	return method, password, nil
}

func normalizeShadowsocksMethod(method string) string {
	switch method {
	case "chacha20-poly1305":
		return "chacha20-ietf-poly1305"
	case "xchacha20-poly1305":
		return "xchacha20-ietf-poly1305"
	default:
		return method
	}
}

func parseShadowsocksPlugin(plugin string) (string, string) {
	plugin = strings.TrimSpace(plugin)
	if plugin == "" {
		return "", ""
	}

	parts := strings.Split(plugin, ";")
	if len(parts) == 0 {
		return "", ""
	}

	name := parts[0]
	if name == "" {
		return "", ""
	}

	options := make([]string, 0, len(parts)-1)
	for _, part := range parts[1:] {
		if part == "" {
			continue
		}
		options = append(options, part)
	}
	if len(options) == 0 {
		return name, ""
	}
	return name, strings.Join(options, ";") + ";"
}
