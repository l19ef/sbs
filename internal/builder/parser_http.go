package builder

import (
	"fmt"
	"net/url"
)

func parseHTTPLine(line, defaultTag string) (Outbound, error) {
	u, err := url.Parse(line)
	if err != nil {
		return Outbound{}, fmt.Errorf("parse http url: %w", err)
	}

	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return Outbound{}, err
	}

	outbound := Outbound{
		Tag:        parseTag(u.Fragment, defaultTag),
		Type:       "http",
		Server:     host,
		ServerPort: port,
	}

	if u.User != nil {
		outbound.Username = u.User.Username()
		outbound.Password, _ = u.User.Password()
	}

	if u.Scheme == "https" {
		outbound.TLS = map[string]any{
			"enabled": true,
		}
	}

	return outbound, nil
}
