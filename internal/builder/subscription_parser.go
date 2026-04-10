package builder

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var countryPrefixPattern = regexp.MustCompile(`^([A-Z]{2})(?:$|[^A-Za-z])`)
var errNoValidSubscriptionNodes = errors.New("no valid subscription nodes found")

func parseSubscriptionContent(data []byte, tagPrefix string, options BuildOptions) ([]Outbound, error) {
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

func parseSubscriptionText(text, tagPrefix string, options BuildOptions) ([]Outbound, error) {
	if text == "" {
		return nil, fmt.Errorf("subscription is empty")
	}

	lines := strings.Split(text, "\n")
	outbounds := make([]Outbound, 0, len(lines))

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

func parseSubscriptionLine(line, tagPrefix string, index int) (Outbound, bool, error) {
	switch {
	case strings.HasPrefix(line, "ss://"):
		outbound, err := parseShadowsocksLine(line, fallbackTag(tagPrefix, "ss", index))
		if err != nil {
			return Outbound{}, true, fmt.Errorf("shadowsocks: %w", err)
		}
		return outbound, true, nil
	case strings.HasPrefix(line, "trojan://"):
		outbound, err := parseTrojanLine(line, fallbackTag(tagPrefix, "trojan", index))
		if err != nil {
			return Outbound{}, true, fmt.Errorf("trojan: %w", err)
		}
		return outbound, true, nil
	case strings.HasPrefix(line, "vless://"):
		outbound, err := parseVLESSLine(line, fallbackTag(tagPrefix, "vless", index))
		if err != nil {
			return Outbound{}, true, fmt.Errorf("vless: %w", err)
		}
		return outbound, true, nil
	case strings.HasPrefix(line, "vmess://"):
		outbound, err := parseVMessLine(line, fallbackTag(tagPrefix, "vmess", index))
		if err != nil {
			return Outbound{}, true, fmt.Errorf("vmess: %w", err)
		}
		return outbound, true, nil
	case strings.HasPrefix(line, "hysteria2://"), strings.HasPrefix(line, "hy2://"):
		outbound, err := parseHysteria2Line(line, fallbackTag(tagPrefix, "hysteria2", index))
		if err != nil {
			return Outbound{}, true, fmt.Errorf("hysteria2: %w", err)
		}
		return outbound, true, nil
	default:
		return Outbound{}, false, nil
	}
}

func fallbackTag(prefix, protocol string, index int) string {
	if prefix == "" {
		prefix = "subscription"
	}
	return fmt.Sprintf("%s-%s-%d", prefix, protocol, index+1)
}

func postProcessOutbounds(outbounds []Outbound, options BuildOptions) []Outbound {
	filtered := filterOutbounds(outbounds, options)
	filtered = ensureTagUniqueness(filtered)
	if !options.Emojify {
		return filtered
	}

	for i, outbound := range filtered {
		tag := outbound.Tag
		if tag == "" {
			continue
		}
		if emojified, ok := emojifyTag(tag); ok {
			filtered[i].Tag = emojified
		}
	}
	return filtered
}

func filterOutbounds(outbounds []Outbound, options BuildOptions) []Outbound {
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

	filtered := make([]Outbound, 0, len(outbounds))
	for _, outbound := range outbounds {
		tag := outbound.Tag
		outboundType := outbound.Type

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

func ensureTagUniqueness(outbounds []Outbound) []Outbound {
	seen := make(map[string]int, len(outbounds))
	for _, outbound := range outbounds {
		tag := outbound.Tag
		seen[tag]++
	}

	renamed := make(map[string]int, len(outbounds))
	for i := range outbounds {
		tag := outbounds[i].Tag
		if seen[tag] > 1 {
			if renamed[tag] == 0 {
				renamed[tag] = 1
				continue
			}
			for n := renamed[tag] + 1; ; n++ {
				newTag := fmt.Sprintf("%s (%d)", tag, n)
				if seen[newTag] == 0 {
					outbounds[i].Tag = newTag
					seen[newTag] = 1
					renamed[tag] = n
					break
				}
			}
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
