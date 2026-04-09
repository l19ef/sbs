package main

import (
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

			return runServer(cfg)
		},
	}

	serveCmd.Flags().IntVarP(&port, "port", "p", 0, "Port to listen on")
	serveCmd.Flags().StringVar(&tlsCert, "tls-cert", "", "TLS certificate path")
	serveCmd.Flags().StringVar(&tlsKey, "tls-key", "", "TLS private key path")

	generateCmd := &cobra.Command{
		Use:   "generate <template>",
		Short: "Generate config from template",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return generate(args[0])
		},
	}

	var outputPath string
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

		result, err := builder.BuildFromFile(tmpl.Path)
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

func generate(templatePath string) error {
	result, err := builder.BuildFromFile(templatePath)
	if err != nil {
		return fmt.Errorf("build config: %w", err)
	}

	fmt.Print(string(result))
	return nil
}
