package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

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
		port     int
		tlsCert  string
		tlsKey   string
		hostname string
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

			return runServer(cfg, hostname)
		},
	}

	serveCmd.Flags().IntVarP(&port, "port", "p", 0, "Port to listen on")
	serveCmd.Flags().StringVar(&tlsCert, "tls-cert", "", "TLS certificate path")
	serveCmd.Flags().StringVar(&tlsKey, "tls-key", "", "TLS private key path")
	serveCmd.Flags().StringVar(&hostname, "hostname", "", "Hostname to display in printed URLs")

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
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("parse config: unexpected trailing content")
	}

	return &cfg, nil
}

func runServer(cfg *HostConfig, displayHostname string) error {
	if err := validateHostConfig(cfg); err != nil {
		return err
	}

	tokenMap := make(map[string]*TemplateConfig)
	for i := range cfg.Templates {
		tokenMap[cfg.Templates[i].Token] = &cfg.Templates[i]
	}

	lastGoodByToken := make(map[string][]byte, len(tokenMap))
	var cacheMu sync.RWMutex

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
			cacheMu.RLock()
			cached, ok := lastGoodByToken[token]
			cacheMu.RUnlock()
			if !ok {
				http.Error(w, "Generation failed", http.StatusInternalServerError)
				return
			}
			log.Printf("serving last good config for token=%s", token)
			result = append([]byte(nil), cached...)
		} else {
			cacheMu.Lock()
			lastGoodByToken[token] = append([]byte(nil), result...)
			cacheMu.Unlock()
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if _, err := w.Write(result); err != nil {
			log.Printf("write response failed: %v", err)
		}
	})

	var ln net.Listener
	var listenPort int
	var err error

	if cfg.Port != 0 {
		ln, err = net.Listen("tcp", fmt.Sprintf(":%d", cfg.Port))
		if err != nil {
			return fmt.Errorf("listen: %w", err)
		}
		listenPort = cfg.Port
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
		p, err := strconv.Atoi(portStr)
		if err != nil {
			ln.Close()
			return fmt.Errorf("parse port: %w", err)
		}
		listenPort = p
	}

	displayHost := displayHostname
	if displayHostname == "" {
		displayHost = fmt.Sprintf("127.0.0.1:%d", listenPort)
	} else if listenPort != 443 {
		displayHost = fmt.Sprintf("%s:%d", displayHostname, listenPort)
	}
	fmt.Printf("Config server running on https://%s\n", displayHost)
	fmt.Println("URLs:")
	for _, tmpl := range cfg.Templates {
		fmt.Printf("  https://%s/config?token=%s\n", displayHost, tmpl.Token)
	}

	server := &http.Server{Handler: mux}
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ServeTLS(ln, cfg.TLSCert, cfg.TLSKey)
	}()

	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-shutdownCtx.Done():
		log.Println("shutdown signal received, stopping server")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown server: %w", err)
	}

	serveErr := <-errCh
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return serveErr
	}
	return nil
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
	cfg.TLSCert = strings.TrimSpace(cfg.TLSCert)
	if cfg.TLSCert == "" {
		return fmt.Errorf("tls_cert is required")
	}
	cfg.TLSKey = strings.TrimSpace(cfg.TLSKey)
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
	for i := range cfg.Templates {
		tmpl := &cfg.Templates[i]
		tmpl.Path = strings.TrimSpace(tmpl.Path)
		tmpl.Token = strings.TrimSpace(tmpl.Token)

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
