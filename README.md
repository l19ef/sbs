# sbs — sing-box subscriptions

CLI tool for generating sing-box configurations from templates with subscription support.

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

When `--out` is set, output is written to the target file atomically.

### serve

Start config server:

```bash
sbs serve config.json
sbs serve config.json --port 443 --tls-cert cert.pem --tls-key key.pem
```

`config.json` example:

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

Validation rules for effective serve config (JSON + CLI flags):

- `tls_cert` and `tls_key` are required in the final merged config
- `port` must be in range `0..65535` (`0` means random free port)
- at least one template is required
- each template must define non-empty `path` and `token`
- template tokens must be unique

## Template format

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
      "format": "auto",
      "emojify": true,
      "exclude": ["ads"],
      "exclude_protocols": ["hysteria2"]
    }
  ],
  "route": { "final": "proxy" }
}
```

### Subscription options

| Option | Type | Description |
|--------|------|-------------|
| `emojify` | bool | Add country flag emojis to tags |
| `exclude` | string[] | Substrings to exclude by tag |
| `exclude_protocols` | string[] | Protocol types to exclude |
| `encoding` | string | `auto` (default), `plain`, or `base64` |
| `format` | string | `auto` (default), `uri`, or `clash` |

#### Emoji tags

With `"emojify": true`, country codes are converted to flag emojis:

| Tag | Result |
|-----|--------|
| US / Trojan | 🇺🇸 US / Trojan |
| JP-Tokyo | 🇯🇵 JP-Tokyo |
| DE Germany | 🇩🇪 DE Germany |

### Supported protocols

- **Shadowsocks** — `ss://...`
- **VMess** — `vmess://...` (base64 JSON)
- **VLESS** — `vless://...`
- **Trojan** — `trojan://...`
- **Hysteria2** — `hysteria2://...` or `hy2://...`

### Supported subscription formats

- **URI** — newline-separated `ss://`, `trojan://`, `vless://`, `vmess://`, `hysteria2://`
- **Clash YAML** — `proxies:` list (group sections are ignored)
