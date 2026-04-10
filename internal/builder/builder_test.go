package builder

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

	if got := outbounds[0]["type"]; got != "shadowsocks" {
		t.Fatalf("unexpected ss type: %#v", got)
	}
	if got := outbounds[1]["type"]; got != "trojan" {
		t.Fatalf("unexpected trojan type: %#v", got)
	}
	if got := outbounds[2]["type"]; got != "vless" {
		t.Fatalf("unexpected vless type: %#v", got)
	}
	if got := outbounds[3]["type"]; got != "vmess" {
		t.Fatalf("unexpected vmess type: %#v", got)
	}
	if got := outbounds[4]["type"]; got != "hysteria2" {
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

	if got := outbounds[0]["tag"]; got != "🇺🇸 US-SLC / Trojan" {
		t.Fatalf("unexpected emojified US tag: %#v", got)
	}
	if got := outbounds[1]["tag"]; got != "Node Trojan" {
		t.Fatalf("unexpected non-country tag: %#v", got)
	}
	if got := outbounds[2]["tag"]; got != "🇬🇧 UK / Trojan" {
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
	if outbounds[0]["type"] != "vmess" || outbounds[1]["type"] != "trojan" {
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
	if got := outbounds[0]["tag"]; got != "Keep SS" {
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
		if got := outbound["tag"]; got != expectedTags[i] {
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
	if outbounds[0]["type"] != "vmess" || outbounds[1]["type"] != "trojan" {
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
		if outbound["type"] != "shadowsocks" {
			t.Fatalf("unexpected type for %q: %#v", line, outbound)
		}
		if outbound["server"] != "server.example.com" || outbound["server_port"] != 9000 {
			t.Fatalf("unexpected server for %q: %#v", line, outbound)
		}
		if outbound["method"] != "chacha20-ietf-poly1305" || outbound["password"] != "pass" {
			t.Fatalf("unexpected auth fields for %q: %#v", line, outbound)
		}
	}
}

func stringsJoinLines(lines ...string) string {
	return fmt.Sprintf("%s\n", strings.Join(lines, "\n"))
}
