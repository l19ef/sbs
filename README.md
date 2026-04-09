# sbs — Sing-Box Subscription Builder

CLI tool for building Sing-Box configurations from templates with subscription support.

## Installation

```bash
go build ./cmd/sbs
```

## Commands

### generate

Generate config from template:

```bash
sbs generate template.json
sbs generate template.json --out output.json
```

### serve

Start config server:

```bash
sbs serve host.json
sbs serve host.json --port 443 --tls-cert cert.pem --tls-key key.pem
```

## Host Config Format

```json
{
  "tls_cert": "/path/to/cert.pem",
  "tls_key": "/path/to/key.pem",
  "port": 443,
  "templates": [
    {
      "path": "template.json",
      "token": "my-token-123"
    }
  ]
}
```

## Template Format

```json
{
  "outbounds": [
    { "tag": "direct", "type": "direct" },
    { "tag": "proxy", "type": "selector", "subscriptions": ["main"] }
  ],
  "subscriptions": [
    {
      "tag": "main",
      "url": "https://example.com/sub",
      "emojify": true,
      "exclude": ["ads"],
      "exclude_protocols": ["hysteria2"]
    }
  ],
  "route": { "final": "proxy" }
}
```

### Subscription Options

| Option | Type | Description |
|--------|------|-------------|
| `emojify` | bool | Add country flags to tags |
| `exclude` | string[] | Substrings to exclude by tag |
| `exclude_protocols` | string[] | Protocol types to exclude |

### Supported Protocols

- **Shadowsocks** — `ss://...`
- **VMess** — `vmess://...` (base64 JSON)
- **VLESS** — `vless://...`
- **Trojan** — `trojan://...`
- **Hysteria2** — `hysteria2://...` / `hy2://...`

### Emoji Tags

With `emojify: true`, country codes are converted to flag emojis:

| Tag | Result |
|-----|--------|
| `US / Trojan` | 🇺🇸 US / Trojan |
| `JP-Tokyo` | 🇯🇵 JP-Tokyo |
| `DE Germany` | 🇩🇪 DE Germany |
