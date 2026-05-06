package builder

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
)

func parseNaiveLine(line, defaultTag string) (Outbound, error) {
	prefix := "http2://"
	base64Part := strings.TrimPrefix(line, prefix)
	if base64Part == line {
		return Outbound{}, fmt.Errorf("invalid naive url: missing http2:// prefix")
	}

	withoutQuery := strings.SplitN(base64Part, "?", 2)[0]
	decoded, err := base64.StdEncoding.DecodeString(withoutQuery)
	if err != nil {
		return Outbound{}, fmt.Errorf("decode naive credentials: %w", err)
	}

	fakeURL := prefix + string(decoded)
	u, err := url.Parse(fakeURL)
	if err != nil {
		return Outbound{}, fmt.Errorf("parse naive url: %w", err)
	}
	if u.User == nil {
		return Outbound{}, fmt.Errorf("naive url is missing credentials")
	}

	username := u.User.Username()
	password, _ := u.User.Password()

	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return Outbound{}, err
	}

	rawTag := extractFragment(line)
	tag := parseTag(rawTag, defaultTag)

	return Outbound{
		Tag:        tag,
		Type:       "naive",
		Server:     host,
		ServerPort: port,
		Username:   username,
		Password:   password,
		TLS: map[string]any{
			"enabled": true,
		},
	}, nil
}

func extractFragment(line string) string {
	if idx := strings.Index(line, "#"); idx >= 0 {
		return line[idx+1:]
	}
	return ""
}
