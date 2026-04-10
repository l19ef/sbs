package builder

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

func decodeBase64(data []byte) ([]byte, error) {
	return decodeBase64String(string(data))
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

func normalizeSubscriptionText(text string) string {
	text = strings.TrimPrefix(text, "\ufeff")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.TrimSpace(text)
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

func intPtr(value int) *int {
	v := value
	return &v
}
