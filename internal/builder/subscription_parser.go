package builder

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var countryPrefixPattern = regexp.MustCompile(`^([A-Z]{2})(?:$|[^A-Za-z])`)
var errNoValidSubscriptionNodes = errors.New("no valid subscription nodes found")

func decodeBase64(data []byte) ([]byte, error) {
	return decodeBase64String(string(data))
}

func parseSubscriptionContent(data []byte, tagPrefix string, options BuildOptions) ([]map[string]any, error) {
	encoding := strings.ToLower(strings.TrimSpace(options.Encoding))
	if encoding == "" {
		encoding = "auto"
	}

	plainText := normalizeSubscriptionText(string(data))

	switch encoding {
	case "plain":
		return parseSubscriptionText(plainText, tagPrefix, options)
	case "base64":
		decoded, err := decodeBase64(data)
		if err != nil {
			return nil, fmt.Errorf("decode base64 subscription: %w", err)
		}
		return parseSubscriptionText(normalizeSubscriptionText(string(decoded)), tagPrefix, options)
	case "auto":
		outbounds, err := parseSubscriptionText(plainText, tagPrefix, options)
		if err == nil {
			return outbounds, nil
		}
		if !errors.Is(err, errNoValidSubscriptionNodes) {
			return nil, err
		}

		decoded, decodeErr := decodeBase64(data)
		if decodeErr != nil {
			return nil, err
		}
		return parseSubscriptionText(normalizeSubscriptionText(string(decoded)), tagPrefix, options)
	default:
		return nil, fmt.Errorf("unsupported encoding %q (expected auto, plain, or base64)", options.Encoding)
	}
}

func parseSubscriptionText(text, tagPrefix string, options BuildOptions) ([]map[string]any, error) {
	if text == "" {
		return nil, fmt.Errorf("subscription is empty")
	}

	lines := strings.Split(text, "\n")
	outbounds := make([]map[string]any, 0, len(lines))

	for index, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		outbound, recognized, err := parseSubscriptionLine(line, tagPrefix, index)
		if err != nil {
			return nil, fmt.Errorf("parse subscription line %d: %w", index+1, err)
		}
		if recognized {
			outbounds = append(outbounds, outbound)
		}
	}

	if len(outbounds) == 0 {
		return nil, errNoValidSubscriptionNodes
	}

	return postProcessOutbounds(outbounds, options), nil
}

func postProcessOutbounds(outbounds []map[string]any, options BuildOptions) []map[string]any {
	filtered := filterOutbounds(outbounds, options)
	filtered = ensureTagUniqueness(filtered)
	if !options.Emojify {
		return filtered
	}

	for _, outbound := range filtered {
		tag, _ := outbound["tag"].(string)
		if tag == "" {
			continue
		}
		if emojified, ok := emojifyTag(tag); ok {
			outbound["tag"] = emojified
		}
	}
	return filtered
}

func filterOutbounds(outbounds []map[string]any, options BuildOptions) []map[string]any {
	if len(options.ExcludePatterns) == 0 && len(options.ExcludeProtocols) == 0 {
		return outbounds
	}

	excludedProtocols := make(map[string]struct{}, len(options.ExcludeProtocols))
	for _, protocol := range options.ExcludeProtocols {
		protocol = strings.ToLower(strings.TrimSpace(protocol))
		if protocol != "" {
			excludedProtocols[protocol] = struct{}{}
		}
	}

	filtered := make([]map[string]any, 0, len(outbounds))
	for _, outbound := range outbounds {
		tag, _ := outbound["tag"].(string)
		outboundType, _ := outbound["type"].(string)

		if _, excluded := excludedProtocols[strings.ToLower(outboundType)]; excluded {
			continue
		}
		if matchesAnyExcludePattern(tag, options.ExcludePatterns) {
			continue
		}

		filtered = append(filtered, outbound)
	}
	return filtered
}

func ensureTagUniqueness(outbounds []map[string]any) []map[string]any {
	seen := make(map[string]int, len(outbounds))
	for _, outbound := range outbounds {
		tag := anyToString(outbound["tag"])
		seen[tag]++
	}

	renamed := make(map[string]int, len(outbounds))
	for i, outbound := range outbounds {
		tag := anyToString(outbound["tag"])
		if seen[tag] > 1 {
			if renamed[tag] == 0 {
				renamed[tag] = 1
				continue
			}
			for n := renamed[tag] + 1; ; n++ {
				newTag := fmt.Sprintf("%s (%d)", tag, n)
				if seen[newTag] == 0 {
					outbound["tag"] = newTag
					seen[newTag] = 1
					renamed[tag] = n
					break
				}
			}
			outbounds[i] = outbound
		} else {
			renamed[tag] = 0
		}
	}
	return outbounds
}

func matchesAnyExcludePattern(tag string, patterns []string) bool {
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if strings.Contains(tag, pattern) {
			return true
		}
	}
	return false
}

func emojifyTag(tag string) (string, bool) {
	match := countryPrefixPattern.FindStringSubmatch(tag)
	if len(match) != 2 {
		return "", false
	}

	flag, ok := countryCodeToFlag(match[1])
	if !ok {
		return "", false
	}
	if strings.HasPrefix(tag, flag+" ") {
		return tag, true
	}
	return flag + " " + tag, true
}

func countryCodeToFlag(code string) (string, bool) {
	code = strings.ToUpper(code)
	if len(code) != 2 {
		return "", false
	}
	if code == "UK" {
		code = "GB"
	}
	for _, r := range code {
		if r < 'A' || r > 'Z' {
			return "", false
		}
	}
	return string([]rune{
		rune(0x1F1E6 + rune(code[0]) - 'A'),
		rune(0x1F1E6 + rune(code[1]) - 'A'),
	}), true
}

func normalizeSubscriptionText(text string) string {
	text = strings.TrimPrefix(text, "\ufeff")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.TrimSpace(text)
}

func parseSubscriptionLine(line, tagPrefix string, index int) (map[string]any, bool, error) {
	switch {
	case strings.HasPrefix(line, "ss://"):
		outbound, err := parseShadowsocksLine(line, fallbackTag(tagPrefix, "ss", index))
		return outbound, true, err
	case strings.HasPrefix(line, "trojan://"):
		outbound, err := parseTrojanLine(line, fallbackTag(tagPrefix, "trojan", index))
		return outbound, true, err
	case strings.HasPrefix(line, "vless://"):
		outbound, err := parseVLESSLine(line, fallbackTag(tagPrefix, "vless", index))
		return outbound, true, err
	case strings.HasPrefix(line, "vmess://"):
		outbound, err := parseVMessLine(line, fallbackTag(tagPrefix, "vmess", index))
		return outbound, true, err
	case strings.HasPrefix(line, "hysteria2://"), strings.HasPrefix(line, "hy2://"):
		outbound, err := parseHysteria2Line(line, fallbackTag(tagPrefix, "hysteria2", index))
		return outbound, true, err
	default:
		return nil, false, nil
	}
}

func fallbackTag(prefix, protocol string, index int) string {
	if prefix == "" {
		prefix = "subscription"
	}
	return fmt.Sprintf("%s-%s-%d", prefix, protocol, index+1)
}

func decodeBase64String(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("empty base64 string")
	}

	var lastErr error
	for _, encoding := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		decoded, err := encoding.DecodeString(value)
		if err == nil {
			return decoded, nil
		}
		lastErr = err
	}

	if mod := len(value) % 4; mod != 0 {
		padded := value + strings.Repeat("=", 4-mod)
		for _, encoding := range []*base64.Encoding{
			base64.StdEncoding,
			base64.URLEncoding,
		} {
			decoded, err := encoding.DecodeString(padded)
			if err == nil {
				return decoded, nil
			}
			lastErr = err
		}
	}

	return nil, lastErr
}

func parseShadowsocksLine(line, defaultTag string) (map[string]any, error) {
	rest := strings.TrimPrefix(line, "ss://")
	mainPart, fragment := splitFragment(rest)
	mainPart, rawQuery := splitQuery(mainPart)

	outbound := Outbound{
		Tag:   parseTag(fragment, defaultTag),
		Type:  "shadowsocks",
		Extra: map[string]any{},
	}

	credentialsPart, serverPart, err := splitShadowsocksParts(mainPart)
	if err != nil {
		return nil, err
	}

	method, password, err := parseShadowsocksCredentials(credentialsPart)
	if err != nil {
		return nil, err
	}

	host, port, err := splitHostPort(serverPart)
	if err != nil {
		return nil, err
	}

	outbound.Server = host
	outbound.ServerPort = port
	outbound.Extra["method"] = normalizeShadowsocksMethod(method)
	outbound.Extra["password"] = password

	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return nil, fmt.Errorf("parse ss query: %w", err)
	}
	if plugin := query.Get("plugin"); plugin != "" {
		for key, value := range buildShadowsocksPluginExtra(plugin) {
			outbound.Extra[key] = value
		}
	}

	return outbound.ToMap(), nil
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
	if !ok {
		return "", "", fmt.Errorf("invalid ss credentials")
	}
	if method == "" {
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

func buildShadowsocksPluginExtra(plugin string) map[string]any {
	extra := map[string]any{}

	plugin = strings.TrimSpace(plugin)
	if plugin == "" {
		return extra
	}

	parts := strings.Split(plugin, ";")
	if len(parts) == 0 {
		return extra
	}

	name := parts[0]
	if name == "" {
		return extra
	}

	extra["plugin"] = name

	options := make([]string, 0, len(parts)-1)
	for _, part := range parts[1:] {
		if part == "" {
			continue
		}
		options = append(options, part)
	}
	if len(options) > 0 {
		extra["plugin_opts"] = strings.Join(options, ";") + ";"
	}

	return extra
}

func parseTrojanLine(line, defaultTag string) (map[string]any, error) {
	u, err := url.Parse(line)
	if err != nil {
		return nil, fmt.Errorf("parse trojan url: %w", err)
	}
	if u.User == nil {
		return nil, fmt.Errorf("trojan url is missing password")
	}

	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return nil, err
	}

	outbound := Outbound{
		Tag:        parseTag(u.Fragment, defaultTag),
		Type:       "trojan",
		Server:     host,
		ServerPort: port,
		TLS: map[string]any{
			"enabled":  true,
			"insecure": false,
		},
		Extra: map[string]any{
			"password": u.User.Username(),
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
	return outbound.ToMap(), nil
}

func parseVLESSLine(line, defaultTag string) (map[string]any, error) {
	u, err := url.Parse(line)
	if err != nil {
		return nil, fmt.Errorf("parse vless url: %w", err)
	}
	if u.User == nil {
		return nil, fmt.Errorf("vless url is missing uuid")
	}

	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return nil, err
	}

	query := u.Query()
	outbound := Outbound{
		Tag:        parseTag(u.Fragment, defaultTag),
		Type:       "vless",
		Server:     host,
		ServerPort: port,
		Extra: map[string]any{
			"uuid":            u.User.Username(),
			"packet_encoding": firstNonEmpty(query.Get("packetEncoding"), "xudp"),
		},
	}

	if flow := query.Get("flow"); flow != "" {
		outbound.Extra["flow"] = flow
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
	return outbound.ToMap(), nil
}

func parseVMessLine(line, defaultTag string) (map[string]any, error) {
	payload := strings.TrimPrefix(line, "vmess://")
	if payload == "" {
		return nil, fmt.Errorf("empty vmess url")
	}

	decoded, err := decodeBase64String(payload)
	if err != nil {
		return nil, fmt.Errorf("decode vmess base64: %w", err)
	}

	decodedStr := strings.TrimSpace(string(decoded))
	if !strings.HasPrefix(decodedStr, "{") {
		return nil, fmt.Errorf("invalid vmess format: expected JSON")
	}

	return parseVMessJSON(decodedStr, defaultTag)
}

func parseVMessJSON(payload, defaultTag string) (map[string]any, error) {
	var item map[string]any
	if err := json.Unmarshal([]byte(payload), &item); err != nil {
		return nil, fmt.Errorf("decode vmess json: %w", err)
	}

	port, err := parseIntString(anyToString(item["port"]))
	if err != nil {
		return nil, fmt.Errorf("invalid vmess port: %w", err)
	}

	outbound := Outbound{
		Tag:        firstNonEmpty(strings.TrimSpace(anyToString(item["ps"])), defaultTag),
		Type:       "vmess",
		Server:     anyToString(item["add"]),
		ServerPort: port,
		Extra: map[string]any{
			"uuid":            anyToString(item["id"]),
			"security":        normalizeVMessSecurity(anyToString(item["scy"])),
			"alter_id":        parseOptionalInt(anyToString(item["aid"])),
			"packet_encoding": "xudp",
		},
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
	return outbound.ToMap(), nil
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

func parseHysteria2Line(line, defaultTag string) (map[string]any, error) {
	line = strings.Replace(line, "hy2://", "hysteria2://", 1)
	u, err := url.Parse(line)
	if err != nil {
		return nil, fmt.Errorf("parse hysteria2 url: %w", err)
	}
	if u.User == nil {
		return nil, fmt.Errorf("hysteria2 url is missing password")
	}

	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return nil, err
	}

	query := u.Query()
	outbound := Outbound{
		Tag:        parseTag(u.Fragment, defaultTag),
		Type:       "hysteria2",
		Server:     host,
		ServerPort: port,
		TLS: map[string]any{
			"enabled":  true,
			"insecure": false,
		},
		Extra: map[string]any{
			"password":  u.User.Username(),
			"up_mbps":   parseDefaultInt(query.Get("upmbps"), 10),
			"down_mbps": parseDefaultInt(query.Get("downmbps"), 100),
		},
	}

	tls := outbound.TLS
	if sni := firstNonEmpty(query.Get("sni"), query.Get("peer")); sni != "" && !strings.EqualFold(sni, "none") {
		tls["server_name"] = sni
	} else {
		tls["insecure"] = true
	}
	if query.Get("insecure") == "1" || strings.EqualFold(query.Get("insecure"), "true") || query.Get("allowInsecure") == "1" {
		tls["insecure"] = true
	}
	if alpn := parseCSVList(firstNonEmpty(query.Get("alpn"), "h3")); len(alpn) > 0 {
		tls["alpn"] = alpn
	}

	if obfsType := query.Get("obfs"); obfsType != "" && obfsType != "none" {
		outbound.Extra["obfs"] = map[string]any{
			"type":     obfsType,
			"password": query.Get("obfs-password"),
		}
	}

	return outbound.ToMap(), nil
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

func shouldEnableTLS(query url.Values) bool {
	security := strings.ToLower(query.Get("security"))
	return security != "" && security != "none" || query.Get("tls") == "1"
}

func parseTag(fragment, fallback string) string {
	if fragment == "" {
		return fallback
	}
	if tag, err := url.QueryUnescape(fragment); err == nil && tag != "" {
		return tag
	}
	return fallback
}

func splitFragment(value string) (string, string) {
	before, after, found := strings.Cut(value, "#")
	if !found {
		return value, ""
	}
	return before, after
}

func splitQuery(value string) (string, string) {
	before, after, found := strings.Cut(value, "?")
	if !found {
		return value, ""
	}
	return before, after
}

func splitHostPort(value string) (string, int, error) {
	host, portString, err := net.SplitHostPort(value)
	if err != nil {
		return "", 0, fmt.Errorf("invalid host:port %q: %w", value, err)
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port %q: %w", portString, err)
	}
	return strings.Trim(host, "[]"), port, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func parseCSVList(value string) []string {
	value = strings.Trim(value, "{}")
	if value == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseOptionalInt(value string) int {
	number, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return number
}

func parseDefaultInt(value string, fallback int) int {
	if value == "" {
		return fallback
	}

	var digits bytes.Buffer
	for _, r := range value {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	if digits.Len() == 0 {
		return fallback
	}

	number, err := strconv.Atoi(digits.String())
	if err != nil {
		return fallback
	}
	return number
}

func parseIntString(value string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(value))
}

func anyToString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case json.Number:
		return typed.String()
	default:
		return fmt.Sprint(value)
	}
}

func splitEarlyDataPath(path string) (string, int) {
	before, after, found := strings.Cut(path, "?ed=")
	if !found {
		return path, 0
	}
	size, err := strconv.Atoi(after)
	if err != nil {
		return before, 0
	}
	return before, size
}

func splitEarlyDataOnlyPath(path string) string {
	before, _ := splitEarlyDataPath(path)
	return before
}
