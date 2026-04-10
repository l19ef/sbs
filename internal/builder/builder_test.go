package builder

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildExpandsSubscriptions(t *testing.T) {
	template := []byte(`{
  "outbounds": [
    {
      "tag": "proxy",
      "type": "selector",
      "subscriptions": ["abc"]
    },
    {
      "tag": "fastest",
      "type": "urltest",
      "subscriptions": ["abc"],
      "url": "http://cp.cloudflare.com/generate_204"
    },
    {
      "tag": "direct",
      "type": "direct"
    }
  ],
  "subscriptions": [
    {
      "tag": "abc",
      "url": "https://example.com/abc"
    }
  ]
}`)

	result, err := Build(template, t.TempDir(), stubLoader{
		payloads: map[string]string{
			"abc": stringsJoinLines(
				"vmess://eyJhZGQiOiJ2bWVzcy5leGFtcGxlLmNvbSIsImFpZCI6IjAiLCJob3N0IjoiY2RuLmV4YW1wbGUuY29tIiwiaWQiOiIxMTExMTExMS0xMTExLTExMTEtMTExMS0xMTExMTExMTExMTExIiwibmV0Ijoid3MiLCJwYXRoIjoiL3dzIiwicG9ydCI6IjQ0MyIsInBzIjoibm9kZS1hIiwic2N5IjoiYXV0byIsInNuaSI6InZtZXNzLmV4YW1wbGUuY29tIiwidGxzIjoidGxzIn0=",
				"trojan://secret@example.com:443#node-b",
			),
		},
	})
	if err != nil {
		t.Fatalf("build config: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatalf("decode result: %v", err)
	}

	if _, ok := decoded["subscriptions"]; ok {
		t.Fatalf("subscriptions should be removed from output")
	}

	outbounds := decoded["outbounds"].([]any)
	if len(outbounds) != 5 {
		t.Fatalf("unexpected outbound count: got %d want 5", len(outbounds))
	}

	proxy := outbounds[0].(map[string]any)
	if _, ok := proxy["subscriptions"]; ok {
		t.Fatalf("subscriptions key should be removed from selector")
	}
	if got := proxy["outbounds"].([]any); len(got) != 2 || got[0].(string) != "node-a" || got[1].(string) != "node-b" {
		t.Fatalf("selector outbounds mismatch: %#v", got)
	}

	fastest := outbounds[1].(map[string]any)
	if got := fastest["outbounds"].([]any); len(got) != 2 {
		t.Fatalf("urltest outbounds mismatch: %#v", got)
	}
}

func TestBuildMergesExistingOutbounds(t *testing.T) {
	template := []byte(`{
  "outbounds": [
    {
      "tag": "proxy",
      "type": "selector",
      "outbounds": ["manual"],
      "subscriptions": ["abc"]
    }
  ],
  "subscriptions": [
    {
      "tag": "abc",
      "url": "https://example.com/abc"
    }
  ]
}`)

	result, err := Build(template, t.TempDir(), stubLoader{
		payloads: map[string]string{
			"abc": "vmess://eyJhZGQiOiJ2bWVzcy5leGFtcGxlLmNvbSIsImFpZCI6IjAiLCJob3N0IjoiY2RuLmV4YW1wbGUuY29tIiwiaWQiOiIxMTExMTExMS0xMTExLTExMTEtMTExMS0xMTExMTExMTExMTExIiwibmV0Ijoid3MiLCJwYXRoIjoiL3dzIiwicG9ydCI6IjQ0MyIsInBzIjoibm9kZS1hIiwic2N5IjoiYXV0byIsInNuaSI6InZtZXNzLmV4YW1wbGUuY29tIiwidGxzIjoidGxzIn0=",
		},
	})
	if err != nil {
		t.Fatalf("build config: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatalf("decode result: %v", err)
	}

	outbounds := decoded["outbounds"].([]any)
	proxy := outbounds[0].(map[string]any)
	got := proxy["outbounds"].([]any)
	if len(got) != 2 || got[0].(string) != "manual" || got[1].(string) != "node-a" {
		t.Fatalf("merged selector outbounds mismatch: %#v", got)
	}
}

func TestBuildExpandsRealURLSubscription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, stringsJoinLines(
			"ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTpwYXNz@server.example.com:9000#Node%20SS",
			"trojan://secret@example.com:443?type=ws&host=cdn.example.com&path=%2Fws&sni=example.com#Node%20Trojan",
		))
	}))
	defer server.Close()

	template := []byte(`{
  "outbounds": [
    {
      "tag": "proxy",
      "type": "selector",
      "subscriptions": ["remote"]
    }
  ],
  "subscriptions": [
    {
      "tag": "remote",
      "url": "` + server.URL + `"
    }
  ]
}`)

	result, err := Build(template, t.TempDir(), DefaultLoader())
	if err != nil {
		t.Fatalf("build config: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatalf("decode result: %v", err)
	}

	outbounds := decoded["outbounds"].([]any)
	if len(outbounds) != 3 {
		t.Fatalf("unexpected outbound count: got %d want 3", len(outbounds))
	}

	selector := outbounds[0].(map[string]any)
	got := selector["outbounds"].([]any)
	if len(got) != 2 || got[0].(string) != "Node SS" || got[1].(string) != "Node Trojan" {
		t.Fatalf("selector outbounds mismatch: %#v", got)
	}

	ssOutbound := outbounds[1].(map[string]any)
	if ssOutbound["type"].(string) != "shadowsocks" || ssOutbound["tag"].(string) != "Node SS" {
		t.Fatalf("unexpected shadowsocks outbound: %#v", ssOutbound)
	}

	trojanOutbound := outbounds[2].(map[string]any)
	if trojanOutbound["type"].(string) != "trojan" || trojanOutbound["tag"].(string) != "Node Trojan" {
		t.Fatalf("unexpected trojan outbound: %#v", trojanOutbound)
	}
}

func TestBuildExpandsMultipleSubscriptions(t *testing.T) {
	template := []byte(`{
  "outbounds": [
    {
      "tag": "proxy",
      "type": "selector",
      "subscriptions": ["one", "two"]
    }
  ],
  "subscriptions": [
    {
      "tag": "one",
      "url": "https://example.com/one"
    },
    {
      "tag": "two",
      "url": "https://example.com/two"
    }
  ]
}`)

	result, err := Build(template, t.TempDir(), stubLoader{
		payloads: map[string]string{
			"one": "vmess://eyJhZGQiOiJ2bWVzcy5leGFtcGxlLmNvbSIsImFpZCI6IjAiLCJob3N0IjoiY2RuLmV4YW1wbGUuY29tIiwiaWQiOiIxMTExMTExMS0xMTExLTExMTEtMTExMS0xMTExMTExMTExMTExIiwibmV0Ijoid3MiLCJwYXRoIjoiL3dzIiwicG9ydCI6IjQ0MyIsInBzIjoibm9kZS1hIiwic2N5IjoiYXV0byIsInNuaSI6InZtZXNzLmV4YW1wbGUuY29tIiwidGxzIjoidGxzIn0=",
			"two": "trojan://secret@example.com:443#node-b",
		},
	})
	if err != nil {
		t.Fatalf("build config: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatalf("decode result: %v", err)
	}

	outbounds := decoded["outbounds"].([]any)
	selector := outbounds[0].(map[string]any)
	got := selector["outbounds"].([]any)
	if len(got) != 2 || got[0].(string) != "node-a" || got[1].(string) != "node-b" {
		t.Fatalf("selector outbounds mismatch: %#v", got)
	}
}

type stubLoader struct {
	payloads map[string]string
}

func (l stubLoader) Load(_ context.Context, source subscriptionSource) ([]byte, error) {
	payload, ok := l.payloads[source.Tag]
	if !ok {
		return nil, fmt.Errorf("unexpected subscription tag %q", source.Tag)
	}
	return []byte(payload), nil
}

func TestParseSubscriptionContentParsesSupportedProtocols(t *testing.T) {
	content := stringsJoinLines(
		"ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTpwYXNz@server.example.com:9000#Node%20SS",
		"trojan://secret@example.com:443?type=ws&host=cdn.example.com&path=%2Fws&sni=example.com#Node%20Trojan",
		"vless://11111111-1111-1111-1111-111111111111@example.com:443/ws?type=ws&security=tls&sni=example.com&host=cdn.example.com&path=%2Fws#Node%20VLESS",
		"vmess://eyJhZGQiOiJ2bWVzcy5leGFtcGxlLmNvbSIsImFpZCI6IjAiLCJob3N0IjoiY2RuLmV4YW1wbGUuY29tIiwiaWQiOiIyMjIyMjIyMi0yMjIyLTIyMjItMjIyMi0yMjIyMjIyMjIyMjIiLCJuZXQiOiJ3cyIsInBhdGgiOiIvd2MiLCJwb3J0IjoiNDQzIiwicHMiOiJOb2RlIFZNZXNzIiwic2N5IjoiYXV0byIsInNuaSI6InZtZXNzLmV4YW1wbGUuY29tIiwidGxzIjoidGxzIn0=",
		"hysteria2://password@example.com:443?sni=example.com#Node%20HY2",
	)

	outbounds, err := parseSubscriptionContent([]byte(content), "remote", BuildOptions{})
	if err != nil {
		t.Fatalf("parse subscription content: %v", err)
	}

	if len(outbounds) != 5 {
		t.Fatalf("unexpected outbound count: got %d want 5", len(outbounds))
	}

	if got := outbounds[0].Type; got != "shadowsocks" {
		t.Fatalf("unexpected ss type: %#v", got)
	}
	if got := outbounds[1].Type; got != "trojan" {
		t.Fatalf("unexpected trojan type: %#v", got)
	}
	if got := outbounds[2].Type; got != "vless" {
		t.Fatalf("unexpected vless type: %#v", got)
	}
	if got := outbounds[3].Type; got != "vmess" {
		t.Fatalf("unexpected vmess type: %#v", got)
	}
	if got := outbounds[4].Type; got != "hysteria2" {
		t.Fatalf("unexpected hysteria2 type: %#v", got)
	}
}

func TestParseSubscriptionContentEmojifiesRecognizedCountryPrefix(t *testing.T) {
	outbounds, err := parseSubscriptionContent([]byte(stringsJoinLines(
		"trojan://secret@example.com:443#US-SLC%20/%20Trojan",
		"trojan://secret@example.com:443#Node%20Trojan",
		"trojan://secret@example.com:443#UK%20/%20Trojan",
	)), "remote", BuildOptions{Emojify: true})
	if err != nil {
		t.Fatalf("parse subscription content: %v", err)
	}

	if got := outbounds[0].Tag; got != "🇺🇸 US-SLC / Trojan" {
		t.Fatalf("unexpected emojified US tag: %#v", got)
	}
	if got := outbounds[1].Tag; got != "Node Trojan" {
		t.Fatalf("unexpected non-country tag: %#v", got)
	}
	if got := outbounds[2].Tag; got != "🇬🇧 UK / Trojan" {
		t.Fatalf("unexpected emojified UK tag: %#v", got)
	}
}

func TestParseSubscriptionContentDecodesBase64Payload(t *testing.T) {
	content := base64.StdEncoding.EncodeToString([]byte(
		"vmess://eyJhZGQiOiJ2bWVzcy5leGFtcGxlLmNvbSIsImFpZCI6IjAiLCJob3N0IjoiY2RuLmV4YW1wbGUuY29tIiwiaWQiOiIxMTExMTExMS0xMTExLTExMTEtMTExMS0xMTExMTExMTExMTExIiwibmV0Ijoid3MiLCJwYXRoIjoiL3dzIiwicG9ydCI6IjQ0MyIsInBzIjoibm9kZS1hIiwic2N5IjoiYXV0byIsInNuaSI6InZtZXNzLmV4YW1wbGUuY29tIiwidGxzIjoidGxzIn0=\n" +
			"trojan://secret@example.com:443#node-b\n",
	))

	outbounds, err := parseSubscriptionContent([]byte(content), "remote", BuildOptions{Encoding: "base64"})
	if err != nil {
		t.Fatalf("parse subscription content: %v", err)
	}

	if len(outbounds) != 2 {
		t.Fatalf("unexpected outbound count: got %d want 2", len(outbounds))
	}
	if outbounds[0].Type != "vmess" || outbounds[1].Type != "trojan" {
		t.Fatalf("unexpected outbound types: %#v", outbounds)
	}
}

func TestParseSubscriptionContentExcludesByPatternAndProtocol(t *testing.T) {
	outbounds, err := parseSubscriptionContent([]byte(stringsJoinLines(
		"trojan://secret@example.com:443#US-SLC%20/%20Trojan",
		"vmess://eyJhZGQiOiJ2bWVzcy5leGFtcGxlLmNvbSIsImFpZCI6IjAiLCJob3N0IjoiY2RuLmV4YW1wbGUuY29tIiwiaWQiOiIyMjIyMjIyMi0yMjIyLTIyMjItMjIyMi0yMjIyMjIyMjIyMjIiLCJuZXQiOiJ3cyIsInBhdGgiOiIvd2MiLCJwb3J0IjoiNDQzIiwicHMiOiJOT0RFIExFR0FDWSIsInNjeSI6ImF1dG8iLCJzbmkiOiJ2bWVzcy5leGFtcGxlLmNvbSIsInRscyI6InRscyJ9",
		"ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTpwYXNz@server.example.com:9000#Keep%20SS",
	)), "remote", BuildOptions{
		ExcludePatterns:  []string{"LEGACY", "US-SLC"},
		ExcludeProtocols: []string{"vmess"},
	})
	if err != nil {
		t.Fatalf("parse subscription content: %v", err)
	}

	if len(outbounds) != 1 {
		t.Fatalf("unexpected outbound count: got %d want 1", len(outbounds))
	}
	if got := outbounds[0].Tag; got != "Keep SS" {
		t.Fatalf("unexpected remaining outbound: %#v", got)
	}
}

func TestParseSubscriptionContentUniquifiesDuplicateTags(t *testing.T) {
	outbounds, err := parseSubscriptionContent([]byte(stringsJoinLines(
		"vless://uuid@1.1.1.1:443?sni=example.com&security=reality&pbk=key1&sid=abc#%F0%9F%9A%80%20Marz%20%28max%29%20%5BVLESS%20-%20tcp%5D",
		"vless://uuid@1.1.1.1:444?sni=example2.com&security=reality&pbk=key2&sid=def#%F0%9F%9A%80%20Marz%20%28max%29%20%5BVLESS%20-%20tcp%5D",
		"trojan://pass@2.2.2.2:443#Node%20A",
		"trojan://pass@2.2.2.2:444#Node%20A",
		"trojan://pass@2.2.2.2:445#Node%20A",
	)), "remote", BuildOptions{})
	if err != nil {
		t.Fatalf("parse subscription content: %v", err)
	}

	if len(outbounds) != 5 {
		t.Fatalf("unexpected outbound count: got %d want 5", len(outbounds))
	}

	expectedTags := []string{
		"🚀 Marz (max) [VLESS - tcp]",
		"🚀 Marz (max) [VLESS - tcp] (2)",
		"Node A",
		"Node A (2)",
		"Node A (3)",
	}
	for i, outbound := range outbounds {
		if got := outbound.Tag; got != expectedTags[i] {
			t.Fatalf("outbound[%d] tag: got %q want %q", i, got, expectedTags[i])
		}
	}
}

func TestParseSubscriptionContentAutoDecodesBase64Payload(t *testing.T) {
	content := base64.StdEncoding.EncodeToString([]byte(
		"vmess://eyJhZGQiOiJ2bWVzcy5leGFtcGxlLmNvbSIsImFpZCI6IjAiLCJob3N0IjoiY2RuLmV4YW1wbGUuY29tIiwiaWQiOiIxMTExMTExMS0xMTExLTExMTEtMTExMS0xMTExMTExMTExMTExIiwibmV0Ijoid3MiLCJwYXRoIjoiL3dzIiwicG9ydCI6IjQ0MyIsInBzIjoibm9kZS1hIiwic2N5IjoiYXV0byIsInNuaSI6InZtZXNzLmV4YW1wbGUuY29tIiwidGxzIjoidGxzIn0=\n" +
			"trojan://secret@example.com:443#node-b\n",
	))

	outbounds, err := parseSubscriptionContent([]byte(content), "remote", BuildOptions{})
	if err != nil {
		t.Fatalf("parse subscription content: %v", err)
	}

	if len(outbounds) != 2 {
		t.Fatalf("unexpected outbound count: got %d want 2", len(outbounds))
	}
	if outbounds[0].Type != "vmess" || outbounds[1].Type != "trojan" {
		t.Fatalf("unexpected outbound types: %#v", outbounds)
	}
}

func TestParseSubscriptionContentRejectsInvalidEncodingOption(t *testing.T) {
	_, err := parseSubscriptionContent([]byte("trojan://secret@example.com:443#n1\n"), "remote", BuildOptions{Encoding: "gzip"})
	if err == nil {
		t.Fatalf("expected unsupported encoding error")
	}
}

func TestParseShadowsocksLineSupportsSIP002Formats(t *testing.T) {
	tests := []string{
		"ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTpwYXNz@server.example.com:9000#NodeA",
		"ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTpwYXNzQHNlcnZlci5leGFtcGxlLmNvbTo5MDAw#NodeB",
		"ss://chacha20-ietf-poly1305:pass@server.example.com:9000#NodeC",
	}

	for _, line := range tests {
		outbound, err := parseShadowsocksLine(line, "fallback")
		if err != nil {
			t.Fatalf("parseShadowsocksLine(%q): %v", line, err)
		}
		got := outboundAsMap(t, outbound)
		if got["type"] != "shadowsocks" {
			t.Fatalf("unexpected type for %q: %#v", line, got)
		}
		if got["server"] != "server.example.com" || asInt(t, got["server_port"]) != 9000 {
			t.Fatalf("unexpected server for %q: %#v", line, got)
		}
		if got["method"] != "chacha20-ietf-poly1305" || got["password"] != "pass" {
			t.Fatalf("unexpected auth fields for %q: %#v", line, got)
		}
	}
}

func TestOutboundToMapIncludesProtocolFields(t *testing.T) {
	outbound := Outbound{
		Tag:            "node-a",
		Type:           "vless",
		Server:         "example.com",
		ServerPort:     443,
		UUID:           "11111111-1111-1111-1111-111111111111",
		PacketEncoding: "xudp",
		Password:       "secret",
		Method:         "chacha20-ietf-poly1305",
		Plugin:         "obfs-local",
		PluginOpts:     "obfs=http;",
		Flow:           "xtls-rprx-vision",
		Security:       "auto",
		AlterID:        intPtr(0),
		UpMbps:         intPtr(10),
		DownMbps:       intPtr(100),
		Obfs: map[string]any{
			"type": "salamander",
		},
		TLS: map[string]any{
			"enabled": true,
		},
		Transport: map[string]any{
			"type": "ws",
		},
	}

	got := outboundAsMap(t, outbound)
	if got["type"] != "vless" || got["server"] != "example.com" || asInt(t, got["server_port"]) != 443 {
		t.Fatalf("missing common fields: %#v", got)
	}
	if got["uuid"] != "11111111-1111-1111-1111-111111111111" || got["packet_encoding"] != "xudp" || got["password"] != "secret" {
		t.Fatalf("missing protocol fields: %#v", got)
	}
}

func TestParseVLESSLineBuildsExpectedOutbound(t *testing.T) {
	line := "vless://11111111-1111-1111-1111-111111111111@example.com:443?type=ws&security=tls&sni=example.com&host=cdn.example.com&path=%2Fws&packetEncoding=packetaddr#Node%20VLESS"

	outbound, err := parseVLESSLine(line, "fallback")
	if err != nil {
		t.Fatalf("parseVLESSLine: %v", err)
	}
	got := outboundAsMap(t, outbound)

	if got["type"] != "vless" || got["tag"] != "Node VLESS" {
		t.Fatalf("unexpected base fields: %#v", got)
	}
	if got["server"] != "example.com" || asInt(t, got["server_port"]) != 443 {
		t.Fatalf("unexpected server fields: %#v", got)
	}
	if got["uuid"] != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("unexpected uuid: %#v", got)
	}
	if got["packet_encoding"] != "packetaddr" {
		t.Fatalf("unexpected packet_encoding: %#v", got)
	}

	tls, ok := got["tls"].(map[string]any)
	if !ok || tls["enabled"] != true || tls["server_name"] != "example.com" {
		t.Fatalf("unexpected tls: %#v", got["tls"])
	}

	transport, ok := got["transport"].(map[string]any)
	if !ok || transport["type"] != "ws" || transport["path"] != "/ws" {
		t.Fatalf("unexpected transport: %#v", got["transport"])
	}
	headers, ok := transport["headers"].(map[string]any)
	if !ok || headers["Host"] != "cdn.example.com" {
		t.Fatalf("unexpected transport headers: %#v", transport)
	}
}

func TestParseVLESSLineRealityAddsTLSBlocks(t *testing.T) {
	line := "vless://11111111-1111-1111-1111-111111111111@reality.example:443?security=reality&pbk=pubkey123&sid=abcd&fp=chrome#Reality"

	outbound, err := parseVLESSLine(line, "fallback")
	if err != nil {
		t.Fatalf("parseVLESSLine: %v", err)
	}
	got := outboundAsMap(t, outbound)

	tls, ok := got["tls"].(map[string]any)
	if !ok {
		t.Fatalf("tls block missing: %#v", got)
	}
	reality, ok := tls["reality"].(map[string]any)
	if !ok || reality["enabled"] != true || reality["public_key"] != "pubkey123" || reality["short_id"] != "abcd" {
		t.Fatalf("unexpected reality block: %#v", tls)
	}
	utls, ok := tls["utls"].(map[string]any)
	if !ok || utls["enabled"] != true || utls["fingerprint"] != "chrome" {
		t.Fatalf("unexpected utls block: %#v", tls)
	}
}

func TestParseShadowsocksPlugin(t *testing.T) {
	plugin, pluginOpts := parseShadowsocksPlugin("obfs-local;obfs=http;obfs-host=example.com")
	if plugin != "obfs-local" {
		t.Fatalf("unexpected plugin field: %q", plugin)
	}
	if pluginOpts != "obfs=http;obfs-host=example.com;" {
		t.Fatalf("unexpected plugin_opts field: %q", pluginOpts)
	}
}

func TestBuildVMessTransportWSWithEarlyData(t *testing.T) {
	transport := buildVMessTransport("ws", map[string]any{
		"host": "cdn.example.com",
		"path": "/ws?ed=2048",
	})
	if transport == nil || transport["type"] != "ws" {
		t.Fatalf("unexpected transport: %#v", transport)
	}
	if transport["path"] != "/ws" || transport["max_early_data"] != 2048 {
		t.Fatalf("unexpected ws transport fields: %#v", transport)
	}
	headers, ok := transport["headers"].(map[string]any)
	if !ok || headers["Host"] != "cdn.example.com" {
		t.Fatalf("unexpected ws headers: %#v", transport)
	}
}

func TestParseHysteria2LineSetsInsecureWhenSNIMissing(t *testing.T) {
	outbound, err := parseHysteria2Line("hysteria2://pass@example.com:443#Node", "fallback")
	if err != nil {
		t.Fatalf("parseHysteria2Line: %v", err)
	}
	tls, ok := outboundAsMap(t, outbound)["tls"].(map[string]any)
	if !ok {
		t.Fatalf("tls block missing: %#v", outbound)
	}
	if tls["insecure"] != true {
		t.Fatalf("expected insecure=true when SNI missing: %#v", tls)
	}
}

func TestProtocolParsingFixture(t *testing.T) {
	content := stringsJoinLines(
		"ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTpwYXNz@server.example.com:9000?plugin=obfs-local%3Bobfs%3Dhttp%3Bobfs-host%3Dcdn.example.com#Node%20SS",
		"trojan://secret@example.com:443?type=ws&host=cdn.example.com&path=%2Fws&sni=example.com#Node%20Trojan",
		"vless://11111111-1111-1111-1111-111111111111@example.com:443?type=grpc&security=reality&pbk=pubkey123&sid=abcd&fp=chrome&serviceName=grpc-vless#Node%20VLESS",
		"vmess://eyJhZGQiOiJ2bWVzcy5leGFtcGxlLmNvbSIsImFpZCI6IjAiLCJob3N0IjoiY2RuLmV4YW1wbGUuY29tIiwiaWQiOiIyMjIyMjIyMi0yMjIyLTIyMjItMjIyMi0yMjIyMjIyMjIyMjIiLCJuZXQiOiJ3cyIsInBhdGgiOiIvd2M/ZWQ9MTAyNCIsInBvcnQiOiI0NDMiLCJwcyI6Ik5vZGUgVk1lc3MiLCJzY3kiOiJhdXRvIiwic25pIjoidm1lc3MuZXhhbXBsZS5jb20iLCJ0bHMiOiJ0bHMifQ==",
		"hysteria2://password@example.com:443?sni=example.com&obfs=salamander&obfs-password=obfspass#Node%20HY2",
	)

	outbounds, err := parseSubscriptionContent([]byte(content), "remote", BuildOptions{})
	if err != nil {
		t.Fatalf("parse subscription content: %v", err)
	}

	actual, err := json.MarshalIndent(toOutboundMaps(outbounds), "", "  ")
	if err != nil {
		t.Fatalf("marshal actual: %v", err)
	}
	actual = append(actual, '\n')

	expectedPath := filepath.Join("testdata", "expected", "protocol_parsing.json")
	expected, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("read expected fixture: %v", err)
	}
	if string(actual) != string(expected) {
		t.Fatalf("protocol parsing output mismatch with fixture")
	}
}

func TestXeovoSampleFixture(t *testing.T) {
	samplePath := filepath.Join("testdata", "subscriptions", "xeovo_sample.txt")
	content, err := os.ReadFile(samplePath)
	if err != nil {
		t.Fatalf("read sample subscription: %v", err)
	}

	outbounds, err := parseSubscriptionContent(content, "remote", BuildOptions{})
	if err != nil {
		t.Fatalf("parse sample subscription: %v", err)
	}

	actual, err := json.MarshalIndent(toOutboundMaps(outbounds), "", "  ")
	if err != nil {
		t.Fatalf("marshal sample output: %v", err)
	}
	actual = append(actual, '\n')

	expectedPath := filepath.Join("testdata", "expected", "xeovo_sample.json")
	expected, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("read expected fixture: %v", err)
	}
	if string(actual) != string(expected) {
		t.Fatalf("xeovo sample output mismatch with fixture")
	}
}

func stringsJoinLines(lines ...string) string {
	return fmt.Sprintf("%s\n", strings.Join(lines, "\n"))
}

func toOutboundMaps(items []Outbound) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		data, err := json.Marshal(item)
		if err != nil {
			panic(err)
		}
		entry := map[string]any{}
		if err := json.Unmarshal(data, &entry); err != nil {
			panic(err)
		}
		out = append(out, entry)
	}
	return out
}

func outboundAsMap(t *testing.T, item Outbound) map[string]any {
	t.Helper()
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal outbound: %v", err)
	}
	entry := map[string]any{}
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("unmarshal outbound: %v", err)
	}
	return entry
}

func asInt(t *testing.T, value any) int {
	t.Helper()
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	default:
		t.Fatalf("expected numeric value, got %T (%v)", value, value)
		return 0
	}
}
