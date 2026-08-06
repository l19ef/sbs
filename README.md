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
  "subscriptions": [
    { "tag": "alice-main", "url": "https://example.com/sub-alice" },
    { "tag": "bob-main",   "url": "https://example.com/sub-bob" },
    { "tag": "shared",     "url": "https://example.com/sub-shared", "emojify": true }
  ],
  "templates": [
    {
      "tag": "Alice",
      "path": "template.json",
      "token": "token-alice",
      "subscriptions": ["alice-main", "shared"]
    },
    {
      "tag": "Bob",
      "path": "template.json",
      "token": "token-bob",
      "subscriptions": ["bob-main"]
    }
  ]
}
```

On startup, the server prints a URL for each template:

```
Config server running on https://example.com
URLs:
  Alice: https://example.com/config?token=token-alice
  Bob:   https://example.com/config?token=token-bob
```

If a build fails, the server logs the error and continues serving the last successfully generated config for that token.

Validation rules for effective serve config (JSON + CLI flags):

- `tls_cert` and `tls_key` are required in the final merged config
- `port` must be in range `0..65535` (`0` means random free port)
- each subscription must define a non-empty `tag` and `url`; tags must be unique
- at least one template is required
- each template must define non-empty `tag`, `path`, and `token`; tokens must be unique
- subscription tags listed in a template must be defined in the top-level `subscriptions`

## Template format

```json
{
  "outbounds": [
    { "tag": "direct", "type": "direct" },
    { "tag": "proxy",  "type": "selector", "subscriptions": ["*"] }
  ],
  "route": { "final": "proxy" }
}
```

The `subscriptions` field on an outbound accepts a list of subscription tags to pull nodes from. The special tag `"*"` expands to all subscriptions available for the current client (defined in the server config or in the template itself), in definition order.

Subscriptions can also be defined directly in the template — useful with `generate`:

```json
{
  "outbounds": [
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
  ]
}
```

If a tag is defined both in the template and in the serve config for the same client, the build fails with an error.

Outbounds can also be defined inline using a `link` field:

```json
{ "link": "trojan://secret@example.com:443#My Node" }
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
- **Naive** — `http2://...`
- **SOCKS5** — `socks5://...` or `socks://...`
- **HTTP/HTTPS** — `http://...` or `https://...`

### Supported subscription formats

- **URI** — newline-separated protocol links, optionally base64-encoded
- **Clash YAML** — `proxies:` list (group sections are ignored)
