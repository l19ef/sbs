package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

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
	Path             string   `json:"path"`
	Emojify          bool     `json:"emojify"`
	Exclude          []string `json:"exclude"`
	ExcludeProtocols []string `json:"exclude_protocols"`
	Token            string   `json:"token"`
}

func main() {
	var (
		port    int
		tlsCert string
		tlsKey  string
	)

	hostCmd := &cobra.Command{
		Use:   "host",
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

			return runServer(cfg)
		},
	}

	hostCmd.Flags().IntVarP(&port, "port", "p", 0, "Port to listen on")
	hostCmd.Flags().StringVar(&tlsCert, "tls-cert", "", "TLS certificate path")
	hostCmd.Flags().StringVar(&tlsKey, "tls-key", "", "TLS private key path")

	var (
		outputPath       string
		emojify          bool
		exclude          string
		excludeProtocols string
	)

	generateCmd := &cobra.Command{
		Use:   "generate <template>",
		Short: "Generate config from template",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return generate(args[0], outputPath, emojify, exclude, excludeProtocols)
		},
	}
	generateCmd.Flags().StringVarP(&outputPath, "out", "o", "", "Output path (defaults to stdout)")
	generateCmd.Flags().BoolVarP(&emojify, "emojify", "e", false, "Add country flags to tags")
	generateCmd.Flags().StringVar(&exclude, "exclude", "", "Substrings to exclude by tag")
	generateCmd.Flags().StringVar(&excludeProtocols, "exclude-protocols", "", "Protocols to exclude, e.g. vmess,hysteria2")

	rootCmd := &cobra.Command{
		Use: "sbs",
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}
	rootCmd.AddCommand(hostCmd)
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
	tokenMap := make(map[string]*TemplateConfig)
	for i := range cfg.Templates {
		if cfg.Templates[i].Token == "" {
			return fmt.Errorf("template %s: token is required", cfg.Templates[i].Path)
		}
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

		result, err := builder.BuildFromFileWithOptions(tmpl.Path, builder.BuildOptions{
			Emojify:          tmpl.Emojify,
			ExcludePatterns:  tmpl.Exclude,
			ExcludeProtocols: tmpl.ExcludeProtocols,
		})
		if err != nil {
			log.Printf("generation failed: %v", err)
			http.Error(w, "Generation failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(result)
	})

	var ln net.Listener
	var displayPort string
	var err error

	if cfg.Port != 0 {
		ln, err = net.Listen("tcp", fmt.Sprintf(":%d", cfg.Port))
		displayPort = fmt.Sprintf(":%d", cfg.Port)
	} else {
		ln, err = net.Listen("tcp", ":0")
		_, portStr, err := net.SplitHostPort(ln.Addr().String())
		if err != nil {
			ln.Close()
			return fmt.Errorf("parse addr: %w", err)
		}
		displayPort = ":" + portStr
	}
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	fmt.Printf("Config server running on https://%s\n", displayPort)
	fmt.Println("URLs:")
	for _, tmpl := range cfg.Templates {
		fmt.Printf("  https://%s/config?token=%s\n", displayPort, tmpl.Token)
	}

	return http.ServeTLS(ln, mux, cfg.TLSCert, cfg.TLSKey)
}

func generate(templatePath, outPath string, emojify bool, exclude, excludeProtocols string) error {
	result, err := builder.BuildFromFileWithOptions(templatePath, builder.BuildOptions{
		Emojify:          emojify,
		ExcludePatterns:  splitCommaSeparated(exclude),
		ExcludeProtocols: splitCommaSeparated(excludeProtocols),
	})
	if err != nil {
		return fmt.Errorf("build config: %w", err)
	}

	if outPath == "" {
		fmt.Print(string(result))
		return nil
	}

	return os.WriteFile(outPath, result, 0o644)
}

func generateToken(length int) string {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		log.Fatalf("failed to generate token: %v", err)
	}
	return hex.EncodeToString(bytes)
}

func splitCommaSeparated(value string) []string {
	if value == "" {
		return nil
	}

	parts := split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = trimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func split(s, sep string) []string {
	if s == "" {
		return nil
	}
	parts := make([]string, 0)
	for {
		i := index(s, sep)
		if i < 0 {
			parts = append(parts, s)
			break
		}
		parts = append(parts, s[:i])
		s = s[i+len(sep):]
	}
	return parts
}

func index(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
