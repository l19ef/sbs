package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateHostConfig(t *testing.T) {
	base := &HostConfig{
		TLSCert: "cert.pem",
		TLSKey:  "key.pem",
		Port:    443,
		Templates: []TemplateConfig{
			{Path: "template.json", Token: "token-a"},
		},
	}

	tests := []struct {
		name    string
		mutate  func(*HostConfig)
		wantErr bool
	}{
		{name: "valid config", mutate: func(*HostConfig) {}, wantErr: false},
		{name: "missing tls cert", mutate: func(cfg *HostConfig) { cfg.TLSCert = "" }, wantErr: true},
		{name: "missing tls key", mutate: func(cfg *HostConfig) { cfg.TLSKey = "" }, wantErr: true},
		{name: "negative port", mutate: func(cfg *HostConfig) { cfg.Port = -1 }, wantErr: true},
		{name: "port too high", mutate: func(cfg *HostConfig) { cfg.Port = 70000 }, wantErr: true},
		{name: "no templates", mutate: func(cfg *HostConfig) { cfg.Templates = nil }, wantErr: true},
		{name: "empty template path", mutate: func(cfg *HostConfig) { cfg.Templates[0].Path = "" }, wantErr: true},
		{name: "empty template token", mutate: func(cfg *HostConfig) { cfg.Templates[0].Token = "" }, wantErr: true},
		{
			name: "duplicate template token",
			mutate: func(cfg *HostConfig) {
				cfg.Templates = append(cfg.Templates, TemplateConfig{Path: "template-2.json", Token: "token-a"})
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := *base
			cfg.Templates = append([]TemplateConfig(nil), base.Templates...)
			tc.mutate(&cfg)

			err := validateHostConfig(&cfg)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestGenerateWritesToOutputPath(t *testing.T) {
	templatePath := writeTemplateFixture(t)
	outputPath := filepath.Join(t.TempDir(), "out.json")

	if err := generate(templatePath, outputPath, nil); err != nil {
		t.Fatalf("generate: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("output file is empty")
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("invalid json output: %v", err)
	}
}

func TestGenerateWritesToStdoutWhenOutputPathIsEmpty(t *testing.T) {
	templatePath := writeTemplateFixture(t)
	var buf bytes.Buffer

	if err := generate(templatePath, "", &buf); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatalf("stdout output is empty")
	}
}

func writeTemplateFixture(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "template.json")
	content := []byte(`{
  "outbounds": [
    {"tag": "direct", "type": "direct"}
  ]
}
`)

	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write template fixture: %v", err)
	}
	return path
}
