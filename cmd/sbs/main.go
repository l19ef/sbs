package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	var (
		port    int
		tlsCert string
		tlsKey  string
	)

	hostCmd := &cobra.Command{
		Use:   "host <config>",
		Short: "Start config server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServer(args[0], port, tlsCert, tlsKey)
		},
	}

	hostCmd.Flags().IntVarP(&port, "port", "p", 443, "Port to listen on")
	hostCmd.Flags().StringVar(&tlsCert, "tls-cert", "", "TLS certificate path")
	hostCmd.Flags().StringVar(&tlsKey, "tls-key", "", "TLS private key path")
	cobra.CheckErr(hostCmd.MarkFlagRequired("tls-cert"))
	cobra.CheckErr(hostCmd.MarkFlagRequired("tls-key"))

	rootCmd := &cobra.Command{Use: "sbs"}
	rootCmd.AddCommand(hostCmd)
	cobra.CheckErr(rootCmd.Execute())
}

func runServer(configPath string, port int, tlsCert, tlsKey string) error {
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	token := generateToken(32)
	addr := fmt.Sprintf(":%d", port)

	mux := http.NewServeMux()
	mux.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		providedToken := r.URL.Query().Get("token")
		if providedToken != token {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(configData)
	})

	fmt.Printf("Config server running on https://localhost:%d\n", port)
	fmt.Printf("Token: %s\n", token)
	fmt.Printf("URL: https://localhost:%d/config?token=%s\n", port, token)

	return http.ListenAndServeTLS(addr, tlsCert, tlsKey, mux)
}

func generateToken(length int) string {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		log.Fatalf("failed to generate token: %v", err)
	}
	return hex.EncodeToString(bytes)
}
