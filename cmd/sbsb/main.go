package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"sb-config-manager/internal/builder"
)

func main() {
	var (
		templatePath     = flag.String("template", "", "Path to the template JSON file")
		outputPath       = flag.String("out", "", "Path to write the generated config JSON (defaults to stdout)")
		emojify          = flag.Bool("emojify", false, "Prefix parsed subscription node tags with a country flag emoji when the tag starts with a country code")
		exclude          = flag.String("exclude", "", "Comma-separated substrings; parsed subscription nodes with matching tags are excluded")
		excludeProtocols = flag.String("exclude-protocols", "", "Comma-separated outbound types to exclude, e.g. vmess,hysteria2")
	)

	flag.Parse()

	if *templatePath == "" {
		fmt.Fprintln(os.Stderr, "error: -template is required")
		os.Exit(2)
	}

	result, err := builder.BuildFromFileWithOptions(*templatePath, builder.BuildOptions{
		Emojify:          *emojify,
		ExcludePatterns:  splitCommaSeparated(*exclude),
		ExcludeProtocols: splitCommaSeparated(*excludeProtocols),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *outputPath == "" {
		fmt.Println(string(result))
		return
	}

	if err := os.WriteFile(*outputPath, result, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing output: %v\n", err)
		os.Exit(1)
	}
}

func splitCommaSeparated(value string) []string {
	if value == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
