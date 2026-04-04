# sbs — sing-box subscriptions

CLI tool for building sing-box configurations from templates with subscriptions support.

## Usage

```bash
go run ./cmd/sbsb -template template.json -out output.json
```

### Options

| Flag | Description |
|------|-------------|
| `-template` | Path to template JSON file (required) |
| `-out` | Output path (defaults to stdout) |
| `-emojify` | Add country flags to tags starting with country codes |
| `-exclude` | Comma-separated substrings to exclude by tag |
| `-exclude-protocols` | Comma-separated protocol types to exclude |

## Template Format

```json
{
  "log": { "level": "info" },
  "dns": { "servers": [{ "type": "tls", "server": "1.1.1.1" }] },
  "inbounds": [{ "type": "tun", "address": "172.16.0.1/30", "auto_route": true }],
  "outbounds": [
    { "tag": "direct", "type": "direct" },
    { "tag": "proxy", "type": "selector", "subscriptions": ["main"] }
  ],
  "subscriptions": [{ "tag": "main", "url": "https://example.com/sub" }],
  "route": { "final": "proxy" }
}
```

### Supported Protocols

- **Shadowsocks** — `ss://...`
- **VMess** — `vmess://...`
- **VLESS** — `vless://...`
- **Trojan** — `trojan://...`
- **Hysteria2** — `hysteria2://...` / `hy2://...`

### Output Example

The builder resolves subscriptions and merges their nodes into the final config:

```json
{
  "log": { "level": "info" },
  "outbounds": [
    { "tag": "direct", "type": "direct" },
    { "tag": "proxy", "type": "selector", "outbounds": ["node-1", "node-2"] },
    { "tag": "node-1", "type": "vmess", "server": "..." },
    { "tag": "node-2", "type": "trojan", "server": "..." }
  ]
}
```

### Emoji Tags

With `-emojify`, country codes are converted to flag emojis:

| Tag | Result |
|-----|--------|
| US / Trojan | 🇺🇸 US / Trojan |
| JP-Tokyo / Shadowsocks | 🇯🇵 JP-Tokyo / Shadowsocks |
| DE | 🇩🇪 DE |
| VLess | VLess |

### Exclude Examples

```bash
sbsb -template template.json -out output.json -exclude-protocols hysteria2,vmess -exclude US,KR -emojify
```

## Subscription Format

Each line must contain a valid URI:

```
ss://base64(method:password)@server:port#Tag
vmess://base64(JSON)
vless://uuid@server:port?params#Tag
trojan://password@server:port?params#Tag
hysteria2://password@server:port?params#Tag
```
