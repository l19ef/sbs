package builder

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type clashSubscription struct {
	Proxies []clashProxy `yaml:"proxies"`
}

type clashProxy struct {
	Name       string                 `yaml:"name"`
	Type       string                 `yaml:"type"`
	Server     string                 `yaml:"server"`
	Port       int                    `yaml:"port"`
	Password   string                 `yaml:"password"`
	Cipher     string                 `yaml:"cipher"`
	UUID       string                 `yaml:"uuid"`
	Network    string                 `yaml:"network"`
	TLS        bool                   `yaml:"tls"`
	SNI        string                 `yaml:"sni"`
	ServerName string                 `yaml:"servername"`
	WSPath     string                 `yaml:"ws-path"`
	WSOpts     clashWSOptions         `yaml:"ws-opts"`
	AlterID    int                    `yaml:"alterId"`
	PluginOpts map[string]interface{} `yaml:"plugin-opts"`
}

type clashWSOptions struct {
	Path    string            `yaml:"path"`
	Headers map[string]string `yaml:"headers"`
}

func parseClashContent(data []byte, tagPrefix string, options BuildOptions) ([]Outbound, error) {
	var doc clashSubscription
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse clash yaml: %w", err)
	}
	if len(doc.Proxies) == 0 {
		return nil, errNoValidSubscriptionNodes
	}

	outbounds := make([]Outbound, 0, len(doc.Proxies))
	for index, proxy := range doc.Proxies {
		outbound, recognized, err := parseClashProxy(proxy, fallbackTag(tagPrefix, "clash", index))
		if err != nil {
			name := strings.TrimSpace(proxy.Name)
			if name == "" {
				name = fmt.Sprintf("proxy #%d", index+1)
			}
			return nil, fmt.Errorf("clash %s: %w", name, err)
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

func parseClashProxy(proxy clashProxy, fallback string) (Outbound, bool, error) {
	typeName := strings.ToLower(strings.TrimSpace(proxy.Type))
	tag := firstNonEmpty(strings.TrimSpace(proxy.Name), fallback)

	switch typeName {
	case "ss", "shadowsocks":
		if proxy.Server == "" || proxy.Port == 0 {
			return Outbound{}, true, fmt.Errorf("shadowsocks requires server and port")
		}
		if proxy.Cipher == "" || proxy.Password == "" {
			return Outbound{}, true, fmt.Errorf("shadowsocks requires cipher and password")
		}
		return Outbound{
			Tag:        tag,
			Type:       "shadowsocks",
			Server:     proxy.Server,
			ServerPort: proxy.Port,
			Method:     normalizeShadowsocksMethod(proxy.Cipher),
			Password:   proxy.Password,
		}, true, nil
	case "trojan":
		if proxy.Server == "" || proxy.Port == 0 {
			return Outbound{}, true, fmt.Errorf("trojan requires server and port")
		}
		if proxy.Password == "" {
			return Outbound{}, true, fmt.Errorf("trojan requires password")
		}
		outbound := Outbound{
			Tag:        tag,
			Type:       "trojan",
			Server:     proxy.Server,
			ServerPort: proxy.Port,
			Password:   proxy.Password,
		}
		if proxy.TLS || proxy.SNI != "" || proxy.ServerName != "" {
			outbound.TLS = map[string]any{
				"enabled":  true,
				"insecure": false,
			}
			if serverName := firstNonEmpty(proxy.SNI, proxy.ServerName); serverName != "" {
				outbound.TLS["server_name"] = serverName
			}
		}
		outbound.Transport = buildClashTransport(proxy)
		return outbound, true, nil
	case "vless":
		if proxy.Server == "" || proxy.Port == 0 {
			return Outbound{}, true, fmt.Errorf("vless requires server and port")
		}
		if proxy.UUID == "" {
			return Outbound{}, true, fmt.Errorf("vless requires uuid")
		}
		outbound := Outbound{
			Tag:            tag,
			Type:           "vless",
			Server:         proxy.Server,
			ServerPort:     proxy.Port,
			UUID:           proxy.UUID,
			PacketEncoding: "xudp",
		}
		if proxy.TLS || proxy.SNI != "" || proxy.ServerName != "" {
			outbound.TLS = map[string]any{
				"enabled":  true,
				"insecure": false,
			}
			if serverName := firstNonEmpty(proxy.SNI, proxy.ServerName); serverName != "" {
				outbound.TLS["server_name"] = serverName
			}
		}
		outbound.Transport = buildClashTransport(proxy)
		return outbound, true, nil
	case "vmess":
		if proxy.Server == "" || proxy.Port == 0 {
			return Outbound{}, true, fmt.Errorf("vmess requires server and port")
		}
		if proxy.UUID == "" {
			return Outbound{}, true, fmt.Errorf("vmess requires uuid")
		}
		outbound := Outbound{
			Tag:            tag,
			Type:           "vmess",
			Server:         proxy.Server,
			ServerPort:     proxy.Port,
			UUID:           proxy.UUID,
			PacketEncoding: "xudp",
			Security:       normalizeVMessSecurity(proxy.Cipher),
			AlterID:        intPtr(proxy.AlterID),
		}
		if proxy.TLS || proxy.SNI != "" || proxy.ServerName != "" {
			outbound.TLS = map[string]any{
				"enabled":  true,
				"insecure": true,
			}
			if serverName := firstNonEmpty(proxy.SNI, proxy.ServerName); serverName != "" {
				outbound.TLS["server_name"] = serverName
			}
		}
		outbound.Transport = buildClashTransport(proxy)
		return outbound, true, nil
	case "hysteria2", "hy2":
		if proxy.Server == "" || proxy.Port == 0 {
			return Outbound{}, true, fmt.Errorf("hysteria2 requires server and port")
		}
		if proxy.Password == "" {
			return Outbound{}, true, fmt.Errorf("hysteria2 requires password")
		}
		outbound := Outbound{
			Tag:        tag,
			Type:       "hysteria2",
			Server:     proxy.Server,
			ServerPort: proxy.Port,
			Password:   proxy.Password,
			UpMbps:     intPtr(10),
			DownMbps:   intPtr(100),
			TLS: map[string]any{
				"enabled":  true,
				"insecure": false,
			},
		}
		if serverName := firstNonEmpty(proxy.SNI, proxy.ServerName); serverName != "" {
			outbound.TLS["server_name"] = serverName
			outbound.TLS["insecure"] = false
		} else {
			outbound.TLS["insecure"] = true
		}
		outbound.TLS["alpn"] = []string{"h3"}
		return outbound, true, nil
	default:
		return Outbound{}, false, nil
	}
}

func buildClashTransport(proxy clashProxy) map[string]any {
	if !strings.EqualFold(proxy.Network, "ws") {
		return nil
	}

	path := firstNonEmpty(proxy.WSOpts.Path, proxy.WSPath, "/")
	transport := map[string]any{
		"type": "ws",
		"path": path,
	}

	host := firstNonEmpty(proxy.WSOpts.Headers["Host"], proxy.WSOpts.Headers["host"], proxy.ServerName)
	if host != "" {
		transport["headers"] = map[string]any{"Host": host}
	}

	return transport
}
