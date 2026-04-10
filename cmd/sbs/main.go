package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"sb-config-manager/internal/builder"
)

type HostConfig struct {
	TLSCert   string           `json:"tls_cert"`
	TLSKey    string           `json:"tls_key"`
	Port      int              `json:"port"`
	Templates []TemplateConfig `json:"templates"`
}

type TemplateConfig struct {
	Path  string `json:"path"`
	Token string `json:"token"`
}

func main() {
	var (
		port    int
		tlsCert string
		tlsKey  string
	)

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start config server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadHostConfig(args[0])
			if err != nil {
				return err
			}

			if tlsCert != "" {
				cfg.TLSCert = tlsCert
			}
			if tlsKey != "" {
				cfg.TLSKey = tlsKey
			}
			if port != 0 {
				cfg.Port = port
			}

			if err := validateHostConfig(cfg); err != nil {
				return err
			}

			return runServer(cfg)
		},
	}

	serveCmd.Flags().IntVarP(&port, "port", "p", 0, "Port to listen on")
	serveCmd.Flags().StringVar(&tlsCert, "tls-cert", "", "TLS certificate path")
	serveCmd.Flags().StringVar(&tlsKey, "tls-key", "", "TLS private key path")

	var outputPath string

	generateCmd := &cobra.Command{
		Use:   "generate <template>",
		Short: "Generate config from template",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return generate(args[0], outputPath, os.Stdout)
		},
	}

	generateCmd.Flags().StringVarP(&outputPath, "out", "o", "", "Output path (defaults to stdout)")

	rootCmd := &cobra.Command{
		Use: "sbs",
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(generateCmd)
	cobra.CheckErr(rootCmd.Execute())
}

func loadHostConfig(path string) (*HostConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg HostConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return &cfg, nil
}

func runServer(cfg *HostConfig) error {
	if err := validateHostConfig(cfg); err != nil {
		return err
	}

	tokenMap := make(map[string]*TemplateConfig)
	for i := range cfg.Templates {
		tokenMap[cfg.Templates[i].Token] = &cfg.Templates[i]
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		tmpl, ok := tokenMap[token]
		if !ok {
			http.Error(w, "Token not found", http.StatusNotFound)
			return
		}

		result, err := builder.BuildFromFile(tmpl.Path)
		if err != nil {
			log.Printf("generation failed: %v", err)
			http.Error(w, "Generation failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if _, err := w.Write(result); err != nil {
			log.Printf("write response failed: %v", err)
		}
	})

	var ln net.Listener
	var displayPort string
	var err error

	if cfg.Port != 0 {
		ln, err = net.Listen("tcp", fmt.Sprintf(":%d", cfg.Port))
		if err != nil {
			return fmt.Errorf("listen: %w", err)
		}
		displayPort = fmt.Sprintf(":%d", cfg.Port)
	} else {
		ln, err = net.Listen("tcp", ":0")
		if err != nil {
			return fmt.Errorf("listen: %w", err)
		}
		_, portStr, err := net.SplitHostPort(ln.Addr().String())
		if err != nil {
			ln.Close()
			return fmt.Errorf("parse addr: %w", err)
		}
		displayPort = ":" + portStr
	}
	fmt.Printf("Config server running on https://%s\n", displayPort)
	fmt.Println("URLs:")
	for _, tmpl := range cfg.Templates {
		fmt.Printf("  https://%s/config?token=%s\n", displayPort, tmpl.Token)
	}

	return http.ServeTLS(ln, mux, cfg.TLSCert, cfg.TLSKey)
}

func generate(templatePath, outputPath string, stdout io.Writer) error {
	result, err := builder.BuildFromFile(templatePath)
	if err != nil {
		return fmt.Errorf("build config: %w", err)
	}

	if outputPath != "" {
		if err := writeAtomically(outputPath, result); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
		return nil
	}

	if stdout == nil {
		stdout = os.Stdout
	}

	if _, err := stdout.Write(result); err != nil {
		return fmt.Errorf("write stdout: %w", err)
	}
	return nil
}

func writeAtomically(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".sbs-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	cleanup = false
	return nil
}

func validateHostConfig(cfg *HostConfig) error {
	if cfg == nil {
		return fmt.Errorf("host config is nil")
	}
	if cfg.TLSCert == "" {
		return fmt.Errorf("tls_cert is required")
	}
	if cfg.TLSKey == "" {
		return fmt.Errorf("tls_key is required")
	}
	if cfg.Port < 0 || cfg.Port > 65535 {
		return fmt.Errorf("port must be in range 0..65535")
	}
	if len(cfg.Templates) == 0 {
		return fmt.Errorf("at least one template is required")
	}

	seenTokens := make(map[string]struct{}, len(cfg.Templates))
	for _, tmpl := range cfg.Templates {
		if tmpl.Path == "" {
			return fmt.Errorf("template path is required")
		}
		if tmpl.Token == "" {
			return fmt.Errorf("template %s: token is required", tmpl.Path)
		}
		if _, exists := seenTokens[tmpl.Token]; exists {
			return fmt.Errorf("duplicate template token %q", tmpl.Token)
		}
		seenTokens[tmpl.Token] = struct{}{}
	}

	return nil
}
