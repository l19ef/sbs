package builder

import (
	"fmt"
	"net/url"
)

func parseSocks5Line(line, defaultTag string) (Outbound, error) {
	u, err := url.Parse(line)
	if err != nil {
		return Outbound{}, fmt.Errorf("parse socks5 url: %w", err)
	}

	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return Outbound{}, err
	}

	outbound := Outbound{
		Tag:        parseTag(u.Fragment, defaultTag),
		Type:       "socks",
		Server:     host,
		ServerPort: port,
	}

	if u.User != nil {
		outbound.Username = u.User.Username()
		outbound.Password, _ = u.User.Password()
	}

	return outbound, nil
}
