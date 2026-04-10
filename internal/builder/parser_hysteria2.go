package builder

import (
	"fmt"
	"net/url"
	"strings"
)

func parseHysteria2Line(line, defaultTag string) (Outbound, error) {
	line = strings.Replace(line, "hy2://", "hysteria2://", 1)
	u, err := url.Parse(line)
	if err != nil {
		return Outbound{}, fmt.Errorf("parse hysteria2 url: %w", err)
	}
	if u.User == nil {
		return Outbound{}, fmt.Errorf("hysteria2 url is missing password")
	}

	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return Outbound{}, err
	}

	query := u.Query()
	outbound := Outbound{
		Tag:        parseTag(u.Fragment, defaultTag),
		Type:       "hysteria2",
		Server:     host,
		ServerPort: port,
		Password:   u.User.Username(),
		UpMbps:     intPtr(parseDefaultInt(query.Get("upmbps"), 10)),
		DownMbps:   intPtr(parseDefaultInt(query.Get("downmbps"), 100)),
		TLS: map[string]any{
			"enabled":  true,
			"insecure": false,
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
		outbound.Obfs = map[string]any{
			"type":     obfsType,
			"password": query.Get("obfs-password"),
		}
	}

	return outbound, nil
}
